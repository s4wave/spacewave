package provider_spacewave

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	packfile_order "github.com/s4wave/spacewave/db/packfile/order"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const syncPushRetryTimeout = 30 * time.Second

const syncNoProgressBackoff = time.Second

const defaultSyncSizeThresholdBytes = 48 * 1024 * 1024

// syncOrderDirtyBlocksLimit bounds optional locality ordering.
const syncOrderDirtyBlocksLimit = 1024

// syncDirtyPageLimit bounds pending metadata acquired in one transaction.
const syncDirtyPageLimit = 4096

// syncController manages packfile push/pull synchronization.
type syncController struct {
	le                *logrus.Entry
	store             kvtx.Store
	client            *SessionClient
	resourceID        string
	mfst              *manifest.Manifest
	lower             *packfile_store.PackfileStore
	remote            func() []*packfile.PackfileEntry
	upper             block.StoreOps
	refGraph          packfile_order.RefGraph
	conf              *SyncConfig
	telemetry         *ProviderAccount
	gateBcast         *broadcast.Broadcast
	skipPull          bool
	remotePullRoutine *coalescedTriggerRoutine

	// dirtySize projects pending bytes under bcast.
	dirtySize int64
	// dirtyPendingAt projects the durable first-pending deadline under bcast.
	dirtyPendingAt time.Time
	// publications tracks mounted hosts with durable pending cloud work.
	publications map[*cloudSOHost]time.Time
	// compactDue records a successful flush since the last merge check, under
	// bcast. Any flush path sets it, so the scheduler merges once the queue drains.
	compactDue bool
	// bcast wakes the scheduler when the durable dirty queue changes.
	bcast broadcast.Broadcast
	// dirtyMtx orders durable dirty mutations and their in-memory projection.
	dirtyMtx csync.Mutex

	// flushMtx serializes flushes and merges, the operations that push packs.
	flushMtx sync.Mutex
	// pullMtx serializes pulls. A pull never waits for a push.
	pullMtx sync.Mutex
	// manifestMtx orders each manifest change with its publication to the lower
	// store, so a pull and a push commit never publish a stale entry set.
	manifestMtx sync.Mutex
	// manifestPublished records the one startup snapshot under manifestMtx.
	manifestPublished bool
	// remoteEntries retains remote snapshot precedence under manifestMtx.
	remoteEntries map[string]*packfile.PackfileEntry
	// compactWaitPull holds merges after a failed merge until a pull refreshes
	// the manifest, so a stale plan is never retried.
	compactWaitPull atomic.Bool
}

// Init reads persisted queue accounting and runs the initial pull. Access-gated pull
// failures are returned so callers can wait for account/resource invalidation
// instead of retrying. Must be called before Execute.
func (s *syncController) Init(ctx context.Context) error {
	if err := s.adoptLegacyPendingUploads(ctx); err != nil {
		return err
	}
	if err := s.updateDirtyState(ctx); err != nil {
		return err
	}

	if s.skipPull {
		s.publishManifest()
		return nil
	}

	if err := s.pull(ctx); err != nil {
		if isCloudAccessGatedError(err) {
			s.le.WithError(err).Warn("initial pull gated, stopping sync")
			return err
		}
		s.le.WithError(err).Warn("initial pull failed")
	}
	return nil
}

// applyManifestDelta records a delta in the manifest and publishes the merged
// entries to the lower store.
func (s *syncController) applyManifestDelta(
	ctx context.Context,
	entries []*packfile.PackfileEntry,
	events []*packfile.PackReplacementEvent,
	cursor uint64,
) error {
	// Commit the delta to the manifest under the publication lock.
	s.manifestMtx.Lock()
	defer s.manifestMtx.Unlock()
	if err := s.mfst.ApplyDelta(ctx, entries, events, cursor); err != nil {
		return err
	}

	// Publish the whole catalog the first time.
	if !s.manifestPublished {
		s.publishManifestLocked()
		return nil
	}

	// Collect the IDs the delta changed.
	ids := make(map[string]bool, len(entries))
	for _, entry := range entries {
		ids[entry.GetId()] = true
	}
	for _, event := range events {
		for _, id := range event.GetReplacedPackIds() {
			ids[id] = true
		}
	}

	// Resolve only changed IDs against the accepted local and remote catalogs.
	var changed []*packfile.PackfileEntry
	var removed []string
	for id := range ids {
		entry := s.remoteEntries[id]
		if entry == nil {
			entry = s.mfst.GetEntry(id)
		}
		if entry == nil {
			removed = append(removed, id)
			continue
		}
		changed = append(changed, entry)
	}
	s.lower.ApplyManifestDelta(changed, removed)
	return nil
}

// publishManifest publishes the merged manifest entries to the lower store.
func (s *syncController) publishManifest() {
	s.manifestMtx.Lock()
	defer s.manifestMtx.Unlock()
	s.publishManifestLocked()
}

// publishManifestLocked replaces the catalog after startup or a remote snapshot change.
// The caller holds manifestMtx.
func (s *syncController) publishManifestLocked() {
	s.lower.UpdateManifest(s.mergedManifestEntries())
	s.manifestPublished = true
}

// mergedManifestEntries joins the remote entries with the local manifest,
// remote first. The caller holds manifestMtx.
func (s *syncController) mergedManifestEntries() []*packfile.PackfileEntry {
	local := s.mfst.GetEntries()
	s.remoteEntries = make(map[string]*packfile.PackfileEntry)
	if s.remote == nil {
		return local
	}
	remote := s.remote()
	if len(remote) == 0 {
		return local
	}
	seen := make(map[string]bool, len(remote)+len(local))
	out := make([]*packfile.PackfileEntry, 0, len(remote)+len(local))
	for _, entry := range remote {
		id := entry.GetId()
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		s.remoteEntries[id] = entry
		out = append(out, entry)
	}
	for _, entry := range local {
		id := entry.GetId()
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, entry)
	}
	return out
}

// pendingSnapshot reads the durable queue projection and its notification together.
func (s *syncController) pendingSnapshot() (time.Time, int64, <-chan struct{}) {
	var first time.Time
	var dirty int64
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		first, dirty, changed = s.dirtyPendingAt, s.dirtySize, getWait()
		for _, pendingAt := range s.publications {
			if first.IsZero() || pendingAt.Before(first) {
				first = pendingAt
			}
		}
	})
	return first, dirty, changed
}

// takeCompactDue clears and reports whether a flush since the last check calls
// for a merge.
func (s *syncController) takeCompactDue() bool {
	var due bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		due, s.compactDue = s.compactDue, false
	})
	return due && !s.conf.GetDisableCompaction()
}

// Execute dispatches from the first pending change, without extending its
// deadline. When ctx ends, Execute drains pending work once before returning.
func (s *syncController) Execute(ctx context.Context) error {
	// Run remote pulls for the life of ctx, then drain pending work.
	if s.remotePullRoutine != nil && !s.skipPull {
		s.remotePullRoutine.SetContext(ctx)
		defer s.remotePullRoutine.ClearContext()
	}
	defer s.drain(ctx)

	// Keep the existing pressure and pack limits independent of the time boundary.
	bo := providerBackoff.Construct()
	threshold := int64(s.conf.GetSizeThresholdBytes())
	if threshold == 0 {
		threshold = defaultSyncSizeThresholdBytes
	}
	interval := time.Duration(s.conf.GetCheckpointIntervalSecs()) * time.Second
	if interval == 0 {
		interval = 30 * time.Second
	}

	// Flush at each deadline or size threshold until ctx ends.
	for ctx.Err() == nil {
		first, dirty, changed := s.pendingSnapshot()
		if first.IsZero() && dirty < threshold {
			bo.Reset()
			if s.takeCompactDue() {
				if err := s.CompactNow(ctx); err != nil && ctx.Err() == nil {
					s.le.WithError(err).Warn("small pack merge failed")
				}
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			}
		}

		// A later edit wakes this wait but keeps the original persisted deadline.
		if delay := time.Until(first.Add(interval)); dirty < threshold && delay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			case <-time.After(delay):
			}
		}

		if err := s.FlushNow(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if isDirtySyncGatedCloudError(err) {
				bo.Reset()
				s.le.WithError(err).Warn("block upload gated, waiting for account state change")
				if err := s.waitDirtySyncGate(ctx); err != nil {
					return nil
				}
				continue
			}
			s.le.WithError(err).Warn("block upload failed")
			_, _, changed = s.pendingSnapshot()
			if err := waitDirtySyncRetry(ctx, changed, nextProviderRetryDelay(bo, err)); err != nil {
				return nil
			}
			continue
		}

		// A flush with no progress must not spin on an expired deadline.
		bo.Reset()
		next, _, changed := s.pendingSnapshot()
		if !next.IsZero() && !next.After(first) {
			if err := waitDirtySyncRetry(ctx, changed, syncNoProgressBackoff); err != nil {
				return nil
			}
		}
	}
	return nil
}

// drain flushes pending blocks and publications once after ctx ends. A
// short-lived mount, such as a CLI write, releases the store before the
// checkpoint deadline; without the drain its edits wait for the next mount. A
// failed drain keeps the durable obligation for that mount.
func (s *syncController) drain(ctx context.Context) {
	// Skip the drain when nothing is pending.
	first, dirty, _ := s.pendingSnapshot()
	if first.IsZero() && dirty == 0 {
		return
	}

	// Flush on a detached context, bounded like a forced sync.
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forceSyncTimeout)
	defer cancel()
	if err := s.FlushNow(flushCtx); err != nil {
		s.le.WithError(err).Warn("pending cloud work kept for the next mount")
	}
}

func waitDirtySyncRetry(ctx context.Context, ch <-chan struct{}, delay time.Duration) error {
	if delay == cbackoff.Stop {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			return nil
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	case <-time.After(delay):
		return nil
	}
}

func (s *syncController) waitDirtySyncGate(ctx context.Context) error {
	for {
		first, dirty, dirtyCh := s.pendingSnapshot()
		if first.IsZero() && dirty == 0 {
			return nil
		}
		if s.gateBcast == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-dirtyCh:
				continue
			}
		}

		var gateCh <-chan struct{}
		s.gateBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			gateCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-dirtyCh:
		case <-gateCh:
			return nil
		}
	}
}

// FlushNow serializes an immediate flush request.
func (s *syncController) FlushNow(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	return s.flushCheckpoint(ctx, true)
}

// FlushNowUnordered flushes dirty blocks without refgraph locality ordering.
func (s *syncController) FlushNowUnordered(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	return s.flushCheckpoint(ctx, false)
}

// PullNow serializes an immediate remote packfile manifest pull. It runs
// alongside a flush or merge.
func (s *syncController) PullNow(ctx context.Context) error {
	if s.skipPull {
		s.publishManifest()
		return nil
	}
	s.pullMtx.Lock()
	defer s.pullMtx.Unlock()
	return s.pull(ctx)
}

// LastPullSequence returns the local manifest's last-seen remote sequence.
func (s *syncController) LastPullSequence(ctx context.Context) (uint64, error) {
	return s.mfst.GetLastPullSequence(ctx)
}

// TriggerRemotePull coalesces a remote block-store nonce notification into one pull.
func (s *syncController) TriggerRemotePull() {
	if s.remotePullRoutine != nil && !s.skipPull {
		s.remotePullRoutine.Trigger()
	}
}

func (s *syncController) pullRemoteOnTrigger(ctx context.Context) {
	if err := s.PullNow(ctx); err != nil && ctx.Err() == nil {
		s.le.WithError(err).Warn("remote nonce pull failed")
		s.recordSyncOwnerError(err)
	}
}

// pushPackfile pushes a packfile and retries the same pack ID once when the
// request was canceled after the worker accepted it.
func (s *syncController) pushPackfile(
	ctx context.Context,
	packID string,
	blockCount int,
	pushFn func(context.Context, string, int) error,
) error {
	err := pushFn(ctx, packID, blockCount)
	if err == nil {
		return nil
	}
	if !isRetryableSyncPushCancel(err) {
		return err
	}

	retryCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		syncPushRetryTimeout,
	)
	defer cancel()

	// note: this fork of logrus dereferences the entry, so le must be set.
	if s.le != nil {
		s.le.WithField("pack-id", packID).
			Debug("retrying canceled sync push with detached context")
	}
	return pushFn(retryCtx, packID, blockCount)
}

// MarkDirty durably schedules written blocks before their caller can
// acknowledge the writes. Repeating a write repairs a failed marker without
// double-counting pending bytes.
func (s *syncController) MarkDirty(ctx context.Context, marks []block_store_writeback.Mark) error {
	// Serialize the committed summary with its scheduler projection.
	release, err := s.dirtyMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	for batch := range slices.Chunk(marks, pendingUploadMutationLimit) {
		var state *PendingUploadState
		err = kvtx.RunTransaction(ctx, true,
			func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, true) },
			func(ctx context.Context, tx kvtx.Tx) error {
				var err error
				state, err = markPendingUploads(ctx, tx, batch)
				return err
			},
		)
		if err != nil {
			return errors.Wrap(err, "retain pending block")
		}

		// Publish each committed batch; retrying a partial mark is idempotent.
		s.publishDirtyState(state)
	}
	return nil
}

// adoptLegacyPendingUploads indexes a queue stored before the summary existed,
// one bounded transaction at a time, before any flush reads the queue.
func (s *syncController) adoptLegacyPendingUploads(ctx context.Context) error {
	release, err := s.dirtyMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	for done := false; !done; {
		err := kvtx.RunTransaction(ctx, true,
			func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, true) },
			func(ctx context.Context, tx kvtx.Tx) error {
				var err error
				done, err = adoptLegacyPendingUploads(ctx, tx)
				return err
			},
		)
		if err != nil {
			return errors.Wrap(err, "adopt pending blocks")
		}
	}
	return nil
}

// updateDirtyState reads the durable summary and acknowledges acquired records.
// The summary is published only after the marker transaction commits.
func (s *syncController) updateDirtyState(ctx context.Context, flushed ...dirtyCandidate) error {
	// Order acknowledgement and initialization with concurrent marking.
	release, err := s.dirtyMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	for {
		var state *PendingUploadState
		write := len(flushed) != 0
		count := min(len(flushed), pendingUploadMutationLimit)
		err = kvtx.RunTransaction(ctx, write,
			func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, write) },
			func(ctx context.Context, tx kvtx.Tx) error {
				var err error
				if write {
					state, err = acknowledgePendingUploads(ctx, tx, flushed[:count])
					return err
				}
				state, err = readPendingUploadState(ctx, tx)
				return err
			},
		)
		if err != nil {
			return errors.Wrap(err, "read pending blocks")
		}

		// Keep each mutation within the browser backend's transaction limit.
		s.publishDirtyState(state)
		flushed = flushed[count:]
		if len(flushed) == 0 {
			break
		}
	}
	return nil
}

// publishDirtyState updates the scheduler and telemetry from committed accounting.
// The caller holds dirtyMtx until the projection has been published.
func (s *syncController) publishDirtyState(state *PendingUploadState) {
	var first time.Time
	if state.GetPendingSinceNanos() != 0 {
		first = time.Unix(0, state.GetPendingSinceNanos())
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.dirtySize, s.dirtyPendingAt = state.GetSizeBytes(), first
		broadcast()
	})
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.setSyncTelemetryPending(id, state.GetSizeBytes(), int(state.GetCount())) //nolint:gosec // the count is bounded by stored queue records, far below MaxInt.
	})
}

// dirtyCandidate is the metadata needed to decide which dirty blocks belong in
// a flush chunk without loading block data into memory.
type dirtyCandidate struct {
	// key identifies the pending block's primary record.
	key []byte
	// hash identifies immutable block contents.
	hash *hash.Hash
	// size is the stored block size used for pack selection.
	size int64
	// sequence identifies this insertion, so stale acknowledgements are harmless.
	sequence uint64
}

// dirtyBlock holds one loaded block with its refs for the currently packed
// flush chunk.
type dirtyBlock struct {
	dirtyCandidate
	stored *block.StoredBlock
}

// preparedSyncChunk is one packed chunk ready to push.
type preparedSyncChunk struct {
	// blocks are the dirty blocks packed into packData. Empty for a merge.
	blocks []dirtyBlock
	// replaces are the committed packs a merge supersedes.
	replaces []string
	// entry is the manifest entry describing the pack.
	entry *packfile.PackfileEntry
	// packData is the encoded pack body.
	packData []byte
	// bodyHash is the SHA-256 digest of packData.
	bodyHash []byte
}

// packBlocks writes the dirty blocks to w and returns the pack result.
func (s *syncController) packBlocks(w io.Writer, blocks []dirtyBlock) (*writer.PackResult, error) {
	idx := 0
	iter := func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		b := blocks[idx]
		idx++
		return b.hash, b.stored, nil
	}

	result, err := writer.PackBlocks(w, iter)
	if err != nil {
		return nil, errors.Wrap(err, "packing blocks")
	}
	return result, nil
}

// cleanupDirtyCandidates removes acknowledged markers and resets an empty queue's deadline atomically.
func (s *syncController) cleanupDirtyCandidates(ctx context.Context, blocks []dirtyCandidate) error {
	return s.updateDirtyState(ctx, blocks...)
}

// orderDirtyBlocks orders dirty block metadata for pack locality before
// loading data.
func (s *syncController) orderDirtyBlocks(ctx context.Context, blocks []dirtyCandidate) ([]dirtyCandidate, error) {
	refs := make([]*block.BlockRef, 0, len(blocks))
	byKey := make(map[string]dirtyCandidate, len(blocks))
	for _, b := range blocks {
		key := b.hash.MarshalString()
		byKey[key] = b
		refs = append(refs, block.NewBlockRef(b.hash))
	}

	orderedRefs, err := packfile_order.BlockRefs(ctx, s.refGraph, refs)
	if err != nil {
		return nil, err
	}
	ordered := make([]dirtyCandidate, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		b, ok := byKey[ref.GetHash().MarshalString()]
		if ok {
			ordered = append(ordered, b)
		}
	}
	return ordered, nil
}

// filterDuplicateDirtyBlocks probes a stable catalog with the shared index cache.
func (s *syncController) filterDuplicateDirtyBlocks(ctx context.Context, view *packfile_store.ManifestSnapshot, blocks []dirtyCandidate) ([]dirtyCandidate, []dirtyCandidate, error) {
	refs := make([]*block.BlockRef, 0, len(blocks))
	for _, b := range blocks {
		refs = append(refs, block.NewBlockRef(b.hash))
	}
	exists, err := view.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, nil, err
	}

	pack := make([]dirtyCandidate, 0, len(blocks))
	deduped := make([]dirtyCandidate, 0)
	for i, b := range blocks {
		if exists[i] {
			deduped = append(deduped, b)
			continue
		}
		pack = append(pack, b)
	}
	if len(deduped) != 0 {
		var bytes int64
		for _, b := range deduped {
			bytes += b.size
		}
		s.le.WithField("dirty-blocks", len(blocks)).
			WithField("deduped-blocks", len(deduped)).
			WithField("deduped-bytes", bytes).
			Debug("filtered duplicate dirty blocks")
		s.telemetrySafeCall(func(t *ProviderAccount, id string) {
			t.addSyncTelemetryDeduped(id, bytes, len(deduped))
		})
	}
	return pack, deduped, nil
}

// scanDirtyPage acquires bounded metadata through a captured insertion cutoff.
// Its iterator and transaction are released before callers read payloads or upload.
func (s *syncController) scanDirtyPage(ctx context.Context, after, through uint64) ([]dirtyCandidate, error) {
	if after >= through {
		return nil, nil
	}
	var candidates []dirtyCandidate
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, false) },
		func(ctx context.Context, tx kvtx.Tx) error {
			// Seek directly to the next queue position without retaining earlier keys.
			iter := tx.Iterate(ctx, []byte(pendingUploadOrderPrefix), true, false)
			defer iter.Close()
			if err := iter.Seek(pendingUploadOrderKey(after + 1)); err != nil {
				return err
			}
			page := make([]dirtyCandidate, 0, syncDirtyPageLimit)
			for iter.Valid() && len(page) < syncDirtyPageLimit {
				keyBytes := iter.Key()
				if len(keyBytes) != len(pendingUploadOrderPrefix)+8 {
					return errors.New("invalid pending upload sequence key")
				}
				sequence := binary.BigEndian.Uint64(keyBytes[len(pendingUploadOrderPrefix):])
				if sequence > through {
					break
				}
				data, err := iter.Value()
				if err != nil {
					return err
				}
				entry := &PendingUploadBlock{}
				if err := entry.UnmarshalVT(data); err != nil {
					return err
				}
				if entry.GetSequence() != sequence {
					return errors.New("pending upload record has a different insertion sequence")
				}
				h := &hash.Hash{}
				if err := h.ParseFromB58(entry.GetHash()); err != nil {
					return err
				}
				key := []byte("dirty/" + entry.GetHash())
				page = append(page, dirtyCandidate{key: key, hash: h, size: entry.GetSizeBytes(), sequence: sequence})
				iter.Next()
			}
			if err := iter.Err(); err != nil {
				return err
			}
			candidates = page
			return nil
		},
	)
	return candidates, err
}

// dirtyCandidateChunkSize treats an unknown size as a full pack candidate.
func dirtyCandidateChunkSize(size int64) int64 {
	if size <= 0 {
		return syncFlushMaxPackBytes
	}
	return size
}

// loadDirtyBlocks loads only the payloads for one bounded pack candidate.
func (s *syncController) loadDirtyBlocks(ctx context.Context, candidates []dirtyCandidate) ([]dirtyBlock, error) {
	blocks := make([]dirtyBlock, 0, len(candidates))
	for _, candidate := range candidates {
		ref := block.NewBlockRef(candidate.hash)
		stored, err := s.upper.GetStoredBlock(ctx, ref)
		if err != nil {
			return nil, errors.Wrap(err, "getting dirty block")
		}
		if stored == nil {
			return nil, errors.Wrap(block.ErrNotFound, candidate.hash.MarshalString())
		}
		if int64(len(stored.Data)) > writer.DefaultMaxPackBytes {
			return nil, errors.Errorf(
				"dirty block %s exceeds max pack chunk size",
				candidate.hash.MarshalString(),
			)
		}
		blocks = append(blocks, dirtyBlock{
			dirtyCandidate: candidate,
			stored:         stored,
		})
	}
	return blocks, nil
}

// flushChunks packs the dirty blocks into chunks on a second goroutine while
// this one pushes and commits the previous chunk, so packing overlaps the
// upload with at most two chunks in memory. Chunks commit in order.
func (s *syncController) flushChunks(ctx context.Context, through uint64, orderBlocks bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	chunks := make(chan *preparedSyncChunk)
	prepared := make(chan error, 1)
	go func() {
		defer close(chunks)
		prepared <- s.prepareChunks(ctx, through, orderBlocks, func(chunk *preparedSyncChunk) error {
			select {
			case chunks <- chunk:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	for chunk := range chunks {
		err := s.pushPreparedChunk(ctx, chunk)
		if err == nil {
			err = s.commitPushedChunk(ctx, chunk)
		}
		if err != nil {
			// Stop the packer and wait for it before returning.
			cancel()
			for range chunks {
			}
			return err
		}
	}
	return <-prepared
}

// prepareChunks fills payload chunks across metadata pages before emitting them.
// Later insertions fall beyond through and belong to the next flush.
func (s *syncController) prepareChunks(
	ctx context.Context,
	through uint64,
	orderBlocks bool,
	emit func(*preparedSyncChunk) error,
) error {
	// Keep only one partial chunk alongside the current candidate page.
	maxBlocks := int(writer.DefaultMaxBlocksPerPack)
	pending := make([]dirtyCandidate, 0, maxBlocks)
	var pendingBytes int64
	flushPending := func() error {
		loaded, err := s.loadDirtyBlocks(ctx, pending)
		if err != nil {
			return err
		}
		if err := s.prepareLoadedBlocks(loaded, emit); err != nil {
			return err
		}
		pending, pendingBytes = pending[:0], 0
		return nil
	}

	// Hold catalog membership fixed while sharing resident indexes across pages.
	view := s.lower.SnapshotManifest()

	// Batch duplicate checks after releasing each metadata transaction.
	for after := uint64(0); after < through; {
		page, err := s.scanDirtyPage(ctx, after, through)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		after = page[len(page)-1].sequence
		blocks, duplicates, err := s.filterDuplicateDirtyBlocks(ctx, view, page)
		if err != nil {
			return errors.Wrap(err, "filtering duplicate dirty blocks")
		}
		if len(duplicates) != 0 {
			if err := s.cleanupDirtyCandidates(ctx, duplicates); err != nil {
				return err
			}
		}
		if orderBlocks {
			blocks, err = s.orderDirtyBlocks(ctx, blocks)
			if err != nil {
				return err
			}
		}

		// Page boundaries never force an extra undersized upload.
		for _, candidate := range blocks {
			size := dirtyCandidateChunkSize(candidate.size)
			if size > writer.DefaultMaxPackBytes {
				return errors.Errorf("dirty block %s exceeds max pack chunk size", candidate.hash.MarshalString())
			}
			if len(pending) != 0 && pendingBytes+size > syncFlushMaxPackBytes {
				if err := flushPending(); err != nil {
					return err
				}
			}
			pending = append(pending, candidate)
			pendingBytes += size
			if len(pending) == maxBlocks {
				if err := flushPending(); err != nil {
					return err
				}
			}
		}
	}
	if len(pending) == 0 {
		return nil
	}
	return flushPending()
}

// prepareLoadedBlocks packs loaded dirty blocks, halving a set whose pack
// exceeds the sync pack target, and passes each pack to emit.
func (s *syncController) prepareLoadedBlocks(blocks []dirtyBlock, emit func(*preparedSyncChunk) error) error {
	chunk, err := s.prepareFlushChunk(blocks)
	if err != nil || chunk == nil {
		return err
	}
	size := int64(len(chunk.packData))
	if size > syncFlushMaxPackBytes && len(blocks) > 1 {
		chunk = nil
		mid := len(blocks) / 2
		if err := s.prepareLoadedBlocks(blocks[:mid], emit); err != nil {
			return err
		}
		return s.prepareLoadedBlocks(blocks[mid:], emit)
	}
	if size > writer.DefaultMaxPackBytes {
		return errors.Errorf("dirty pack %s exceeds max pack size", chunk.entry.GetId())
	}
	if size > syncFlushMaxPackBytes {
		s.le.WithField("pack-id", chunk.entry.GetId()).
			WithField("bytes", size).
			WithField("target", syncFlushMaxPackBytes).
			Debug("single-block sync pack exceeds the sync pack target")
	}
	return emit(chunk)
}

// commitPushedChunk records a pushed pack in the manifest and clears the dirty
// markers of its blocks, so a later failure in the same flush does not push the
// same blocks again under a different pack.
func (s *syncController) commitPushedChunk(ctx context.Context, chunk *preparedSyncChunk) error {
	if err := s.applyManifestDelta(ctx, []*packfile.PackfileEntry{chunk.entry}, nil, 0); err != nil {
		return errors.Wrap(err, "applying push delta")
	}

	flushed := make([]dirtyCandidate, len(chunk.blocks))
	for i, block := range chunk.blocks {
		flushed[i] = block.dirtyCandidate
	}
	return s.cleanupDirtyCandidates(ctx, flushed)
}

// prepareFlushChunk packs one bounded dirty-block chunk.
func (s *syncController) prepareFlushChunk(blocks []dirtyBlock) (*preparedSyncChunk, error) {
	var buf bytes.Buffer
	started := time.Now()
	result, err := s.packBlocks(&buf, blocks)
	s.le.WithField("blocks", len(blocks)).
		WithField("duration", time.Since(started)).
		Debug("packed dirty blocks")
	if err != nil {
		return nil, err
	}
	if result.BlockCount == 0 {
		return nil, nil
	}
	packID, err := identity.BuildPackID(s.resourceID, result)
	if err != nil {
		return nil, errors.Wrap(err, "build pack id")
	}

	entry := &packfile.PackfileEntry{
		Id:                 packID,
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
		CreatedAt:          timestamppb.New(time.Now().UTC()),
	}
	return &preparedSyncChunk{
		blocks:   blocks,
		entry:    entry,
		packData: buf.Bytes(),
		bodyHash: result.PackBytesDigest,
	}, nil
}

// pushPreparedChunk uploads one prepared packfile chunk.
func (s *syncController) pushPreparedChunk(ctx context.Context, chunk *preparedSyncChunk) error {
	entry := chunk.entry
	pushBytes := int64(len(chunk.packData))
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.startSyncTelemetryPush(id, pushBytes)
	})
	started := time.Now()
	err := s.pushPackfile(
		ctx,
		entry.GetId(),
		int(entry.GetBlockCount()), //nolint:gosec // prepared entries are produced under writer.DefaultMaxBlocksPerPack (4096).
		func(ctx context.Context, packID string, blockCount int) error {
			return s.client.syncPushDataWithProgress(
				ctx,
				s.resourceID,
				&syncPushPack{
					packID:             packID,
					blockCount:         blockCount,
					bodyHash:           chunk.bodyHash,
					bloomFilter:        entry.GetBloomFilter(),
					bloomFormatVersion: entry.GetBloomFormatVersion(),
					replacedPackIDs:    chunk.replaces,
				},
				chunk.packData,
				func(sent int64) {
					s.telemetrySafeCall(func(t *ProviderAccount, id string) {
						t.setSyncTelemetryPushProgress(id, sent)
					})
				},
			)
		},
	)
	s.le.WithField("pack-id", entry.GetId()).
		WithField("blocks", entry.GetBlockCount()).
		WithField("bytes", len(chunk.packData)).
		WithField("duration", time.Since(started)).
		Debug("pushed packfile")
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.finishSyncTelemetryPush(id, pushBytes, err)
	})
	if err != nil {
		return errors.Wrap(err, "pushing packfile")
	}

	s.le.WithField("pack-id", entry.GetId()).
		WithField("blocks", entry.GetBlockCount()).
		Debug("flushed packfile")
	return nil
}

// flush collects dirty blocks, packs them, pushes to the server, and updates the manifest.
func (s *syncController) flush(ctx context.Context, orderBlocks bool) error {
	// Capture every insertion that must precede the caller's publication.
	var state *PendingUploadState
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, false) },
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			state, err = readPendingUploadState(ctx, tx)
			return err
		},
	)
	if err != nil {
		return err
	}
	if state.GetCount() == 0 {
		return nil
	}

	// Preserve the existing locality-ordering limit and payload pipeline.
	orderBlocks = orderBlocks && state.GetCount() <= syncOrderDirtyBlocksLimit
	s.le.WithField("dirty-blocks", state.GetCount()).
		WithField("order-blocks", orderBlocks).
		WithField("max-chunk-bytes", syncFlushMaxPackBytes).
		Debug("starting dirty block flush")
	return s.flushChunks(ctx, state.GetLastSequence(), orderBlocks)
}

// pull applies the catalog pages after the stored cursor. A restarted pull
// lists the whole catalog, so it drops every pulled pack the listing omits
// and stores its cursor only with the final page. The caller holds pullMtx or
// runs before Execute.
func (s *syncController) pull(ctx context.Context) error {
	since, err := s.mfst.GetLastPullSequence(ctx)
	if err != nil {
		return errors.Wrap(err, "getting last pull sequence")
	}

	// listed holds the packs a restarted pull has listed so far.
	var listed map[string]bool
	for {
		s.telemetrySafeCall(func(t *ProviderAccount, id string) {
			t.startSyncTelemetryPull(id)
		})
		page, err := s.client.SyncPull(ctx, s.resourceID, since)
		s.telemetrySafeCall(func(t *ProviderAccount, id string) {
			t.finishSyncTelemetryPull(id, err)
		})
		if err != nil {
			return errors.Wrap(err, "pulling from server")
		}
		s.compactWaitPull.Store(false)

		// Choose the cursor this page completes. A restarted listing stores
		// none until its final page drops the packs it omitted.
		if page.GetRestart() {
			listed = make(map[string]bool)
		}
		if listed != nil {
			for _, entry := range page.GetEntries() {
				listed[entry.GetId()] = true
			}
		}
		next := page.NextCursor()
		if page.GetMore() && next <= since && !page.GetRestart() {
			return errors.Errorf("catalog page after %d did not advance", since)
		}
		events := page.GetReplacementEvents()
		var cursor uint64
		switch {
		case !page.GetMore():
			cursor = page.GetLatestSequence()
			if dropped := s.unlistedPacks(listed); len(dropped) != 0 {
				events = append(events, &packfile.PackReplacementEvent{ReplacedPackIds: dropped})
			}
		case listed == nil:
			cursor = next
		}

		if err := s.applyManifestDelta(ctx, page.GetEntries(), events, cursor); err != nil {
			return errors.Wrap(err, "applying pull delta")
		}
		s.le.WithField("entries", len(page.GetEntries())).
			WithField("replacement-events", len(events)).
			WithField("restart", page.GetRestart()).
			Debug("pulled packfile page")
		if !page.GetMore() {
			s.recordSyncTelemetryRemoteSequence(page.GetLatestSequence())
			return nil
		}
		since = next
	}
}

// unlistedPacks returns the pulled packs a restarted listing omitted. Locally
// authored entries carry no sequence and stay until a pull replaces them. A
// nil listing omits nothing.
func (s *syncController) unlistedPacks(listed map[string]bool) []string {
	if listed == nil {
		return nil
	}
	var unlisted []string
	for _, entry := range s.mfst.GetEntries() {
		if entry.GetSequence() != 0 && !listed[entry.GetId()] {
			unlisted = append(unlisted, entry.GetId())
		}
	}
	return unlisted
}

func (s *syncController) recordSyncTelemetryRemoteSequence(sequence uint64) {
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.setSyncTelemetryCloudRemoteSequence(id, sequence)
	})
}

// telemetrySafeCall invokes fn against the attached telemetry account when
// telemetry is enabled. The shared resource id and account handle are bound
// so call sites read as a single line per telemetry event.
func (s *syncController) telemetrySafeCall(fn func(t *ProviderAccount, resourceID string)) {
	if s.telemetry == nil {
		return
	}
	fn(s.telemetry, s.resourceID)
}

func (s *syncController) recordSyncOwnerError(err error) {
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.recordSyncTelemetryError(id, err)
	})
}

func isRetryableSyncPushCancel(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "deadline exceeded")
}

// _ is a type assertion
