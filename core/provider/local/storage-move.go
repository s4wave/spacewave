package provider_local

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"golang.org/x/sync/errgroup"
)

const (
	// moveProgressInterval is the number of fetched blocks between progress
	// reports.
	moveProgressInterval = 64
	// moveFetchConcurrency bounds the concurrent block reads of the fetch phase.
	moveFetchConcurrency = 16
)

// errUploaded ends the upload watch once the new placement holds every block.
var errUploaded = errors.New("uploaded")

// MovePhase is a step of a storage move.
type MovePhase int

const (
	// MovePhaseFetch copies blocks held only by the old backend into the
	// account's own storage.
	MovePhaseFetch MovePhase = iota + 1
	// MovePhaseUpload waits for the new backend to hold every block.
	MovePhaseUpload
	// MovePhaseDone reports the completed move.
	MovePhaseDone
)

// MoveProgress is the state of a storage move.
type MoveProgress struct {
	// Phase is the current step.
	Phase MovePhase
	// Fetched is the number of blocks checked during the fetch phase.
	Fetched int
	// Total is the number of blocks the fetch phase checks.
	Total int
	// Upload is the new placement's upload status during the upload phase.
	Upload UploadStatus
}

// ListBlockStoreRefs returns every block the block store holds: the blocks
// reachable in the volume's GC ref graph from the store's bucket node.
func (a *ProviderAccount) ListBlockStoreRefs(ctx context.Context, blockStoreID string) ([]*block.BlockRef, error) {
	rg := a.GetVolume().GetRefGraph()
	if rg == nil {
		return nil, nil
	}
	bucketIRI := block_gc.BucketIRI(BlockStoreBucketID(
		a.t.p.info.GetProviderId(),
		a.t.accountInfo.GetProviderAccountId(),
		blockStoreID,
	))

	// Walk the bucket's roots and the objects and blocks they reference.
	seen := map[string]struct{}{bucketIRI: {}}
	pending := []string{bucketIRI}
	var refs []*block.BlockRef
	for len(pending) != 0 {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		targets, err := rg.GetOutgoingRefs(ctx, node)
		if err != nil {
			return nil, errors.Wrapf(err, "read references of %q", node)
		}
		for _, target := range targets {
			if _, ok := seen[target]; ok || block_gc.IsPermanentRoot(target) {
				continue
			}
			seen[target] = struct{}{}
			pending = append(pending, target)
			if ref, ok := block_gc.ParseBlockIRI(target); ok {
				refs = append(refs, ref)
			}
		}
	}
	return refs, nil
}

// MoveSpaceStorage moves a SharedObject's blocks to a storage backend, or to
// the account's own storage when backendID is empty, and calls fn with the
// progress until the new placement holds every block.
//
// Leaving a backend first copies the blocks only it holds into the account's
// own storage, so the switch never strands a block. Entering a backend queues
// every block for upload, which continues after this call returns early. A
// repeated call resumes an interrupted move. The old backend's objects stay in
// its bucket, where other Spaces placed on it may share them.
func (a *ProviderAccount) MoveSpaceStorage(
	ctx context.Context,
	sharedObjectID string,
	backendID string,
	fn func(MoveProgress) error,
) error {
	blockStoreID := a.lookupSharedObjectBlockStoreID(sharedObjectID)
	if blockStoreID == "" {
		return sobject.ErrSharedObjectNotFound
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	if backendID != "" && settings.FindStorageBackend(backendID) == nil {
		return errors.Wrap(account_settings.ErrStorageBackendNotFound, backendID)
	}

	tkrRef, tkr, _ := a.bstores.AddKeyRef(blockStoreID)
	defer tkrRef.Release()

	current := settings.FindBlockStorePlacement(blockStoreID).GetStorageBackendId()
	if current != backendID {
		if current != "" {
			if err := a.fetchPlacedBlocks(ctx, tkr, fn); err != nil {
				return err
			}
		}
		if err := a.PlaceBlockStore(ctx, blockStoreID, backendID); err != nil {
			return err
		}
	}
	if backendID != "" {
		if err := waitUploaded(ctx, tkr, backendID, fn); err != nil {
			return err
		}
	}
	return fn(MoveProgress{Phase: MovePhaseDone})
}

// lookupSharedObjectBlockStoreID returns the block store of a SharedObject in
// the account, or empty when the account has no such SharedObject.
func (a *ProviderAccount) lookupSharedObjectBlockStoreID(sharedObjectID string) string {
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == sharedObjectID {
			return entry.GetRef().GetBlockStoreId()
		}
	}
	return ""
}

// fetchPlacedBlocks copies every block of the store the account's own storage
// lacks from the backend's bucket or peers, with the refs the source recorded,
// one batch write per chunk.
//
// A block no source holds is skipped: the backend did not hold it either.
func (a *ProviderAccount) fetchPlacedBlocks(ctx context.Context, tkr *bstoreTracker, fn func(MoveProgress) error) error {
	bs, err := tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	refs, err := a.ListBlockStoreRefs(ctx, tkr.id)
	if err != nil {
		return errors.Wrap(err, "list blocks to fetch")
	}

	progress := MoveProgress{Phase: MovePhaseFetch, Total: len(refs)}
	if err := fn(progress); err != nil {
		return err
	}
	local, remote := bs.placement.local, bs.placement.remote
	for chunk := range slices.Chunk(refs, moveProgressInterval) {
		found, err := local.GetBlockExistsBatch(ctx, chunk)
		if err != nil {
			return errors.Wrap(err, "check local blocks")
		}

		// Read the missing blocks concurrently, then store them together.
		entries := make([]*block.PutBatchEntry, len(chunk))
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(moveFetchConcurrency)
		for i, ref := range chunk {
			if found[i] {
				continue
			}
			eg.Go(func() error {
				stored, err := remote.GetStoredBlock(egCtx, ref)
				if err != nil {
					return errors.Wrapf(err, "fetch block %s", ref.MarshalString())
				}
				if stored != nil {
					entries[i] = &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs}
				}
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
		entries = slices.DeleteFunc(entries, func(entry *block.PutBatchEntry) bool {
			return entry == nil
		})
		if len(entries) != 0 {
			if err := local.PutBlockBatch(ctx, entries); err != nil {
				return errors.Wrap(err, "store fetched blocks")
			}
		}

		progress.Fetched += len(chunk)
		if err := fn(progress); err != nil {
			return err
		}
	}
	return nil
}

// waitUploaded reports upload progress until the store is placed on backendID,
// its blocks are queued for upload, and the queue is empty.
func waitUploaded(ctx context.Context, tkr *bstoreTracker, backendID string, fn func(MoveProgress) error) error {
	var prev UploadStatus
	err := watchTrackerUploadStatus(ctx, tkr, func(status UploadStatus) error {
		// Wait for the placement to reach the store and queue its blocks.
		if status.Backend.GetId() != backendID || status.Target != backendID {
			return nil
		}
		if status.Pending == 0 {
			return errUploaded
		}
		if status.Pending == prev.Pending && status.PendingBytes == prev.PendingBytes && status.Err == prev.Err {
			return nil
		}
		prev = status
		return fn(MoveProgress{Phase: MovePhaseUpload, Upload: status})
	})
	if errors.Is(err, errUploaded) {
		return nil
	}
	return err
}
