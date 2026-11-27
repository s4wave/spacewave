package provider_local

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/sync/errgroup"
)

const (
	// moveProgressInterval is the number of fetched blocks between progress
	// reports.
	moveProgressInterval = 64
	// moveFetchConcurrency bounds the concurrent block reads of the fetch phase.
	moveFetchConcurrency = 16
)

// errMoved ends a move's status watch once the store reached its new placement.
var errMoved = errors.New("moved")

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

// ListBlockStoreRefs returns every block the block store holds: the blocks a
// named root of the store's bucket reaches through child lists.
func (a *ProviderAccount) ListBlockStoreRefs(ctx context.Context, blockStoreID string) ([]*block.BlockRef, error) {
	// Resolve the block-store bucket on a volume that keeps child lists.
	walker, ok := a.GetVolume().(interface {
		WalkBucket(ctx context.Context, bucketID string, fn func(*block.BlockRef) error) error
	})
	if !ok {
		return nil, nil
	}
	bucketID := BlockStoreBucketID(
		a.t.p.info.GetProviderId(),
		a.t.accountInfo.GetProviderAccountId(),
		blockStoreID,
	)

	// Walk the bucket's roots and the blocks they reach.
	var refs []*block.BlockRef
	err := walker.WalkBucket(ctx, bucketID, func(ref *block.BlockRef) error {
		refs = append(refs, ref)
		return nil
	})
	return refs, err
}

// MoveSpaceStorage moves a SharedObject's blocks to a storage backend, or to
// the account's own storage when backendID is empty, and calls fn with the
// progress until the new placement holds every block.
//
// Leaving a backend first copies the blocks only it holds into the account's
// own storage, so the switch never strands a block. Entering a backend queues
// every block for upload, which continues after this call returns early. A
// repeated call resumes an interrupted move. The switch releases the store's
// objects on the old backend, which the account then deletes.
func (a *ProviderAccount) MoveSpaceStorage(
	ctx context.Context,
	sharedObjectID string,
	backendID string,
	fn func(MoveProgress) error,
) error {
	// Resolve the shared object's block store and the named storage backend.
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

	// Retain the block-store tracker until the move returns.
	tkrRef, tkr, _ := a.bstores.AddKeyRef(blockStoreID)
	defer tkrRef.Release()

	// Fetch blocks only the old backend holds, then place and wait for the
	// blocks written before the move to upload. Eviction waits until the
	// store leaves the old backend, so no block the fetch found local is
	// dropped from the disk before the switch.
	since := time.Now().Round(0)
	placement := settings.FindBlockStorePlacement(blockStoreID)
	current := placement.GetStorageBackendId()
	if current != backendID {
		if current != "" {
			release, err := tkr.evictGate.Lock(ctx)
			if err != nil {
				return err
			}
			defer release()
			backend := settings.FindStorageBackend(current)
			if backend == nil {
				return errors.Wrap(account_settings.ErrStorageBackendNotFound, current)
			}
			if err := a.fetchPlacedBlocks(ctx, tkr, backend, fn); err != nil {
				return err
			}
		}
		next := &account_settings.BlockStorePlacement{BlockStoreId: blockStoreID, StorageBackendId: backendID}
		if backendID != "" {
			next.Cache = placement.GetCache().CloneVT()
		}
		if err := a.PlaceBlockStore(ctx, next); err != nil {
			return err
		}
	}
	if backendID != "" {
		if err := tkr.waitUploaded(ctx, backendID, since, fn); err != nil {
			return err
		}
	} else if current != "" {
		if err := tkr.waitUnplaced(ctx); err != nil {
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
// lacks from the backend's hot bucket, its cold tier or peers, with the refs
// the source recorded, one batch write per chunk.
//
// The fetch opens its own store on the backend's bucket, so a backend that
// cannot be reached fails the move before the placement changes and releases
// the bucket's objects. It skips only blocks those sources do not hold. An
// unresolved cold read aborts the move. The walk stops at a block without a
// child list, such as a hole or a block a store before format 7 wrote, so the
// fetch also checks each child a fetched block references that the walk did not
// reach: the bucket may hold the only copy.
//
// Cost: one existence check per child of a fetched block, batched per chunk.
// The walk already reached most children, so those children add only a map
// lookup.
func (a *ProviderAccount) fetchPlacedBlocks(
	ctx context.Context,
	tkr *bstoreTracker,
	backend *account_settings.StorageBackend,
	fn func(MoveProgress) error,
) error {
	// Open the local store and the backend bucket, closing the bucket on return.
	bs, err := tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	opened, err := tkr.openBackendStore(ctx, backend)
	if err != nil {
		return errors.Wrap(err, "open storage backend")
	}
	bucket := opened.store
	defer bucket.Close()

	// Refuse to leave the configured backend unless its cold mount is present.
	bucketID := BlockStoreBucketID(a.t.p.info.GetProviderId(), a.t.accountInfo.GetProviderAccountId(), tkr.id)
	cold, _ := bs.placement.cold.(*block_store_s3.ColdPackStore)
	if config := configuredColdPacks.Load(); config != nil && config.config.BlockStoreLocalID == bucketID {
		if config.config.BackendID != backend.GetId() {
			cold = nil // A host-local cold manifest does not follow a move.
		} else if cold == nil {
			return errors.Wrap(block.ErrUnavailable, "cold tier must be mounted before moving storage")
		}
	}

	// Read the opened backend and the old placement's cold tier before peers.
	source := block.StoreOps(bucket)
	fetchConcurrency := moveFetchConcurrency
	if cold != nil {
		source = &fallbackReads{StoreOps: source, fallback: &coldMoveReads{ColdPackStore: cold}}
		fetchConcurrency = 4 // Leave room in the eight-slot lane for user reads.
	}
	remote := block_store.NewStoreReadThrough(
		func() block.StoreOps { return source },
		func() block.StoreOps { return a.getP2PStore(bucketID) },
		nil,
	)
	refs, err := a.ListBlockStoreRefs(ctx, tkr.id)
	if err != nil {
		return errors.Wrap(err, "list blocks to fetch")
	}

	// Note the walk's blocks, so a fetched block's children it reached are
	// not checked twice.
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		seen[ref.MarshalString()] = struct{}{}
	}

	// Report fetch progress.
	progress := MoveProgress{Phase: MovePhaseFetch, Total: len(refs)}
	if err := fn(progress); err != nil {
		return err
	}

	// Hold the copies under a stage until the copy ends: a block may arrive
	// before the parent that counts it.
	local, release, err := block.OpenStage(ctx, bs.placement.local)
	if err != nil {
		return errors.Wrap(err, "open fetch stage")
	}
	defer release()

	// Copy each missing chunk.
	for len(refs) != 0 {
		chunk := refs[:min(len(refs), moveProgressInterval)]
		refs = refs[len(chunk):]
		found, err := local.GetBlockExistsBatch(ctx, chunk)
		if err != nil {
			return errors.Wrap(err, "check local blocks")
		}

		// Read the missing blocks concurrently, then store them together.
		entries := make([]*block.PutBatchEntry, len(chunk))
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(fetchConcurrency)
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
					if !stored.RefsSource.Recorded() {
						// A verified body is not a proof that the block is a leaf.
						return errors.Wrapf(block.ErrRefsUnknown, "fetch block %s", ref.MarshalString())
					}
					entries[i] = &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsSource.Asserted()}
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
			if _, err := local.PutBlockBatch(ctx, entries); err != nil {
				return errors.Wrap(err, "store fetched blocks")
			}
		}

		// Queue the children of the fetched blocks the walk did not reach.
		for _, entry := range entries {
			for _, child := range entry.Refs {
				key := child.MarshalString()
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					refs = append(refs, child)
					progress.Total++
				}
			}
		}

		progress.Fetched += len(chunk)
		if err := fn(progress); err != nil {
			return err
		}
	}
	return nil
}

// coldMoveReads keeps unresolved cold reads from looking like a missing block
// to the storage mover. This adapter is used only for GetStoredBlock.
type coldMoveReads struct {
	// ColdPackStore supplies the placement's immutable pack catalog.
	*block_store_s3.ColdPackStore
}

// GetStoredBlock reports unverified absence as unavailable during a move.
func (s *coldMoveReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return s.GetStoredBlockForMove(ctx, ref)
}

// waitUnplaced waits until the store leaves its backend and stops queueing
// writes for upload.
func (t *bstoreTracker) waitUnplaced(ctx context.Context) error {
	err := t.watchUploadStatus(ctx, func(status UploadStatus) error {
		if status.Backend == nil && !status.Enabled {
			return errMoved
		}
		return nil
	})
	if errors.Is(err, errMoved) {
		return nil
	}
	return err
}

// waitUploaded reports upload progress until the store is placed on backendID,
// its blocks are queued for upload, and every block written before since has
// uploaded. Later writes wait out their settle age without holding the move.
func (t *bstoreTracker) waitUploaded(
	ctx context.Context,
	backendID string,
	since time.Time,
	fn func(MoveProgress) error,
) error {
	var prev UploadStatus
	err := t.watchUploadStatus(ctx, func(status UploadStatus) error {
		// Wait for the placement to reach the store and queue its blocks.
		if status.Backend.GetId() != backendID || status.Target != backendID {
			return nil
		}
		if status.Pending == 0 || !status.Oldest.Before(since) {
			return errMoved
		}
		if status.Pending == prev.Pending && status.PendingBytes == prev.PendingBytes && status.Err == prev.Err {
			return nil
		}
		prev = status
		return fn(MoveProgress{Phase: MovePhaseUpload, Upload: status})
	})
	if errors.Is(err, errMoved) {
		return nil
	}
	return err
}
