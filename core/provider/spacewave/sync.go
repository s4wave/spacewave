package provider_spacewave

import (
	"context"
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_order "github.com/s4wave/spacewave/db/packfile/order"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const syncPushRetryTimeout = 30 * time.Second

const syncNoProgressBackoff = time.Second

// dirtyCorruptRetryMax bounds the wait before a flush reads the dirty blocks
// whose local bytes failed their hash again. New writes do not wait for it:
// they flush one checkpoint interval after they arrive.
const dirtyCorruptRetryMax = 10 * time.Minute

const defaultSyncSizeThresholdBytes = 48 * 1024 * 1024

// syncOrderDirtyBlocksLimit bounds optional locality ordering.
const syncOrderDirtyBlocksLimit = 1024

// syncDirtyPageLimit bounds pending metadata acquired in one transaction.
const syncDirtyPageLimit = 4096

// syncController manages packfile push/pull synchronization.
type syncController struct {
	// le is the logger.
	le *logrus.Entry
	// store holds the durable dirty queue.
	store kvtx.Store
	// client calls the cloud block store.
	client *SessionClient
	// resourceID is the cloud block store ID.
	resourceID string
	// mfst is the local packfile manifest.
	mfst *manifest.Manifest
	// lower serves the packfiles in the manifest.
	lower *packfile_store.PackfileStore
	// remote lists remote packfile entries the manifest does not hold yet.
	remote func() []*packfile.PackfileEntry
	// upper holds the local blocks awaiting upload.
	upper block.StoreOps
	// childLists orders dirty blocks by the volume's child lists.
	childLists packfile_order.ChildLists
	// conf is the sync configuration.
	conf *SyncConfig
	// telemetry receives sync telemetry when set.
	telemetry *ProviderAccount
	// gateBcast fires when account state that gates uploads changes.
	gateBcast *broadcast.Broadcast
	// skipPull disables remote pulls.
	skipPull bool
	// remotePullRoutine runs one coalesced pull per remote nonce trigger.
	remotePullRoutine *coalescedTriggerRoutine
	// remoteNonce is the highest block store sequence the cloud reported.
	remoteNonce atomic.Uint64
	// linked uploads the chunks of linked files, or is nil for a store that
	// links none.
	linked *linkedLane

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
	// hold is the store's held state, under bcast. Only setHold changes it.
	hold holdState
	// writes counts MarkDirty calls under bcast, so a held store waits for a
	// new write rather than for any broadcast.
	writes uint64
	// corruptRecheck makes the next flush read the known corrupt blocks
	// again. It is set by the recheck deadline and by a flush that a write
	// overlapped.
	corruptRecheck atomic.Bool
}

// holdState is a store's held state: the dirty blocks a flush found corrupt.
// The store is held exactly while blocks is not empty; an empty set has a zero
// since and partial false.
type holdState struct {
	// blocks holds the corrupt dirty blocks by hash. A flush skips and counts
	// them without reading them until corruptRecheck is set or a write of the
	// hash removes it. The map is replaced, never changed.
	blocks map[string]struct{}
	// since is when the store first held a block back.
	since time.Time
	// partial is set when the pass that last set blocks stopped before
	// reading every dirty block, so their count is a lower bound. Only the
	// sync status shows it: a flush that stops early returns its own error.
	partial bool
}

// heldError is the result of a flush that uploaded every dirty block it could
// verify and held back count whose local bytes fail their hash. It published
// nothing: a checkpoint is published only when every block it reaches is on
// the remote. It matches block.ErrBlockCorrupt, so a caller that needs every
// block on the remote fails as on any other corrupt read.
type heldError struct {
	// count is the number of corrupt dirty blocks held back.
	count int
	// since is when the store first held a block back.
	since time.Time
}

// Error names the held state the person sees. Only a complete flush returns
// a heldError, so count is exact.
func (e *heldError) Error() string {
	return fmt.Sprintf("sync held: %d damaged blocks on this device, nothing published since %s",
		e.count, e.since.Format(time.RFC3339))
}

// Is matches block.ErrBlockCorrupt.
func (e *heldError) Is(target error) bool {
	return target == block.ErrBlockCorrupt
}

// Init reads persisted queue accounting and runs the initial pull. Access-gated pull
// failures are returned so callers can wait for account/resource invalidation
// instead of retrying. Must be called before Execute.
func (s *syncController) Init(ctx context.Context) error {
	if err := s.updateDirtyState(ctx); err != nil {
		return err
	}

	if s.skipPull {
		s.publishManifest()
		return nil
	}

	if err := s.pull(ctx); err != nil {
		if clouderror.IsAccessGated(err) {
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
	// Return the local manifest alone when there are no remote entries.
	local := s.mfst.GetEntries()
	s.remoteEntries = make(map[string]*packfile.PackfileEntry)
	if s.remote == nil {
		return local
	}
	remote := s.remote()
	if len(remote) == 0 {
		return local
	}

	// Take each remote entry, then each local entry not already taken.
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
	// Read the queue projection and the earliest pending publication.
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
// deadline. When ctx ends, Execute drains pending work once before returning,
// unless the cloud's Retry-After from the last failed flush still runs.
func (s *syncController) Execute(ctx context.Context) error {
	// Run remote pulls for the life of ctx, then drain pending work.
	if s.remotePullRoutine != nil && !s.skipPull {
		s.remotePullRoutine.SetContext(ctx)
		defer s.remotePullRoutine.ClearContext()
	}
	var retryAt time.Time
	defer func() { s.drain(ctx, retryAt) }()

	// Upload linked chunks beside the dirty lane, stopping before the drain.
	if s.linked != nil {
		linkedCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.linked.Execute(linkedCtx)
		}()
		defer func() {
			cancel()
			<-done
		}()
	}

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

	// corruptDelay is the wait before a flush reads the known corrupt dirty
	// blocks again, zero while none are known. It doubles from the checkpoint
	// interval to dirtyCorruptRetryMax, since rereading the blocks fails the
	// same way until something removes them. recheckAt is when that read is
	// due, and rechecked reports that the last flush was that read.
	var corruptDelay time.Duration
	var recheckAt time.Time
	var rechecked bool

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
			//gostyle:timer deadline
			case <-time.After(delay):
			}
		}

		writes, _ := s.writeSnapshot()
		err := s.FlushNow(ctx)

		// A held flush published nothing and waits for a new write, which
		// flushes one interval after it arrives, or for the slow delay to
		// read the held blocks again. Nothing else wakes it, so a held store
		// does no work while idle.
		var held *heldError
		if errors.As(err, &held) {
			bo.Reset()
			retryAt = time.Time{}
			if recheckAt.IsZero() || rechecked {
				corruptDelay = min(max(2*corruptDelay, interval), dirtyCorruptRetryMax)
				recheckAt = time.Now().Add(corruptDelay)
				s.le.WithError(held).WithField("recheck-in", corruptDelay).Warn("sync held")
			}

			// Merge small packs a publication before the hold left due.
			if s.takeCompactDue() {
				if err := s.CompactNow(ctx); err != nil && ctx.Err() == nil {
					s.le.WithError(err).Warn("small pack merge failed")
				}
			}
			recheck, err := s.waitHeld(ctx, writes, recheckAt, interval)
			if err != nil {
				return nil
			}
			if recheck {
				s.corruptRecheck.Store(true)
			}
			rechecked = recheck
			continue
		}
		if err != nil {
			retryAt = time.Now().Add(clouderror.RetryAfter(err))
			if ctx.Err() != nil {
				return nil
			}
			if clouderror.IsAccessGated(err) || clouderror.IsLimitReached(err) {
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

		bo.Reset()
		retryAt = time.Time{}
		corruptDelay, recheckAt, rechecked = 0, time.Time{}, false

		// A flush with no progress must not spin on an expired deadline.
		next, _, changed := s.pendingSnapshot()
		if !next.IsZero() && !next.After(first) {
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
			//gostyle:timer backoff
			case <-time.After(syncNoProgressBackoff):
			}
		}
	}
	return nil
}

// drain flushes pending blocks and publications once after ctx ends. A
// short-lived mount, such as a CLI write, releases the store before the
// checkpoint deadline; without the drain its edits wait for the next mount. A
// failed or skipped drain keeps the durable obligation for that mount. The
// drain is skipped before retryAt, the end of the Retry-After the cloud gave
// the last failed flush, so a rate-limited store is not called early.
func (s *syncController) drain(ctx context.Context, retryAt time.Time) {
	// Skip the drain when nothing is pending or the Retry-After still runs.
	first, dirty, _ := s.pendingSnapshot()
	if first.IsZero() && dirty == 0 || time.Now().Before(retryAt) {
		return
	}

	// Flush on a detached context, bounded like a forced sync.
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forceSyncTimeout)
	defer cancel()

	// A held flush publishes nothing either; the held blocks keep their
	// markers for the next mount.
	if err := s.FlushNow(flushCtx); err != nil {
		s.le.WithError(err).Warn("pending cloud work kept for the next mount")
	}
}

// writeSnapshot returns the write count with the broadcast wait, read together.
func (s *syncController) writeSnapshot() (uint64, <-chan struct{}) {
	var writes uint64
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		writes, changed = s.writes, getWait()
	})
	return writes, changed
}

// waitHeld waits while a flush holds corrupt blocks back. A write after the
// flush started, counted past writes, ends the wait one interval later; the
// recheckAt deadline ends it with recheck set. Other broadcasts, such as a
// flush another caller ran, do not end it. It returns ctx's error when ctx
// ends.
func (s *syncController) waitHeld(ctx context.Context, writes uint64, recheckAt time.Time, interval time.Duration) (bool, error) {
	// Wait for a new write or the recheck deadline.
	deadline := time.NewTimer(time.Until(recheckAt))
	defer deadline.Stop()
	for {
		now, changed := s.writeSnapshot()
		if now != writes {
			break
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return true, nil
		case <-changed:
		}
	}

	// Let a burst of writes gather before the flush.
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(interval):
		return false, nil
	}
}

// waitDirtySyncRetry waits delay after a failed flush. Local edits do not end
// the wait, so editing during an outage or a rate limit does not retry early.
// A stopped backoff waits for the next local edit, on changed, instead.
func waitDirtySyncRetry(ctx context.Context, changed <-chan struct{}, delay time.Duration) error {
	// Wait on changed only when the backoff stopped.
	var timer <-chan time.Time
	if delay != cbackoff.Stop {
		t := time.NewTimer(delay)
		defer t.Stop()
		timer, changed = t.C, nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	case <-timer:
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

// FlushNow serializes an immediate flush request. A flush that held back a
// dirty block whose local bytes fail its hash returns a *heldError and
// publishes nothing; the block keeps its marker and its local copy.
func (s *syncController) FlushNow(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	return s.flushCheckpoint(ctx, true)
}

// FlushNowUnordered flushes dirty blocks without refgraph locality ordering.
// Like FlushNow, it fails with a *heldError while a corrupt block is held.
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

// TriggerRemotePull coalesces a remote block-store nonce notification into one
// pull. The pull is skipped when the last pull already reached the nonce.
func (s *syncController) TriggerRemotePull(nonce uint64) {
	// Record the highest nonce, then wake the pull routine.
	if s.remotePullRoutine == nil || s.skipPull {
		return
	}
	for {
		prev := s.remoteNonce.Load()
		if nonce <= prev || s.remoteNonce.CompareAndSwap(prev, nonce) {
			break
		}
	}
	s.remotePullRoutine.Trigger()
}

// pullRemoteOnTrigger pulls unless the last pull already reached the highest
// reported nonce, as with an echo of this store's own push.
func (s *syncController) pullRemoteOnTrigger(ctx context.Context) {
	if last, err := s.LastPullSequence(ctx); err == nil && s.remoteNonce.Load() <= last {
		return
	}
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
	// Push once and return unless a cancel arrived after the worker accepted.
	err := pushFn(ctx, packID, blockCount)
	if err == nil {
		return nil
	}
	if !isRetryableSyncPushCancel(err) {
		return err
	}

	// Retry the same pack ID once with a detached context.
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

	// Count the write for a held store's wait. A write of a held block's hash
	// takes it out of the known set, so the next flush reads it again. The
	// read finds sound bytes only when the block was removed before the
	// write: a put of a hash the store holds keeps the stored bytes.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.writes++
		s.setHold(withoutMarks(s.hold.blocks, marks), s.hold.partial)
		broadcast()
	})
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

// cleanupDirtyCandidates removes acknowledged markers and resets an empty queue's deadline atomically.
func (s *syncController) cleanupDirtyCandidates(ctx context.Context, blocks []dirtyCandidate) error {
	return s.updateDirtyState(ctx, blocks...)
}

// orderDirtyBlocks orders dirty block metadata for pack locality before
// loading data.
func (s *syncController) orderDirtyBlocks(ctx context.Context, blocks []dirtyCandidate) ([]dirtyCandidate, error) {
	// Index the candidates by hash.
	refs := make([]*block.BlockRef, 0, len(blocks))
	byKey := make(map[string]dirtyCandidate, len(blocks))
	for _, b := range blocks {
		key := b.hash.MarshalString()
		byKey[key] = b
		refs = append(refs, block.NewBlockRef(b.hash))
	}

	// Order their refs for locality and map them back to candidates.
	orderedRefs, err := packfile_order.BlockRefs(ctx, s.childLists, refs)
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
func (s *syncController) filterDuplicateDirtyBlocks(
	ctx context.Context,
	view *packfile_store.ManifestSnapshot,
	blocks []dirtyCandidate,
) ([]dirtyCandidate, []dirtyCandidate, error) {
	// Probe the catalog for every candidate.
	refs := make([]*block.BlockRef, 0, len(blocks))
	for _, b := range blocks {
		refs = append(refs, block.NewBlockRef(b.hash))
	}
	exists, err := view.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, nil, err
	}

	// Split the candidates the catalog already holds from those to pack.
	pack := make([]dirtyCandidate, 0, len(blocks))
	deduped := make([]dirtyCandidate, 0)
	for i, b := range blocks {
		if exists[i] {
			deduped = append(deduped, b)
			continue
		}
		pack = append(pack, b)
	}

	// Report the deduplicated bytes.
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
	// Read one page of queue records after the cursor.
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

// loadDirtyBlocks loads only the payloads for one bounded pack candidate. A
// block whose local bytes fail their hash is left out and added to corrupt:
// only a pushed chunk clears dirty markers, so it stays queued and counted
// while the rest upload. A block in held, the held set the pass started
// from, that reads sound leaves the held set at once, so a pass that stops
// later does not count it.
func (s *syncController) loadDirtyBlocks(ctx context.Context, candidates []dirtyCandidate, held, corrupt map[string]struct{}) ([]dirtyBlock, error) {
	blocks := make([]dirtyBlock, 0, len(candidates))
	for _, candidate := range candidates {
		ref := block.NewBlockRef(candidate.hash)
		stored, err := s.upper.GetStoredBlock(ctx, ref)
		if errors.Is(err, block.ErrBlockCorrupt) {
			corrupt[candidate.hash.MarshalString()] = struct{}{}
			continue
		}
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
		if key := candidate.hash.MarshalString(); hasKey(held, key) {
			s.releaseHold(key)
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
// upload with at most two chunks in memory. Chunks commit in order. It returns
// the count of corrupt dirty blocks the pass held back.
func (s *syncController) flushChunks(ctx context.Context, through uint64, orderBlocks bool) (int, error) {
	// Pack the chunks on a second goroutine.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	chunks := make(chan *preparedSyncChunk)
	prepared := make(chan error, 1)
	var corrupt int
	go func() {
		defer close(chunks)
		var err error
		corrupt, err = s.prepareChunks(ctx, through, orderBlocks, func(chunk *preparedSyncChunk) error {
			select {
			case chunks <- chunk:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		prepared <- err
	}()

	// Push and commit each chunk in order.
	for chunk := range chunks {
		err := s.pushPreparedChunk(ctx, chunk)
		if err == nil {
			err = s.commitPushedChunk(ctx, chunk)
		}
		if err != nil {
			// Stop the packer at its next chunk and wait for it to record
			// the held set of the blocks it read. Nothing receives its
			// chunks now, so it cannot read on past the failure.
			cancel()
			<-prepared
			return 0, err
		}
	}
	err := <-prepared
	return corrupt, err
}

// prepareChunks fills payload chunks across metadata pages before emitting them.
// Later insertions fall beyond through and belong to the next flush. Dirty
// blocks whose local bytes fail their hash stay queued: the pass packs the
// rest and returns their count. A block the last pass found corrupt is
// skipped and counted without a read until corruptRecheck is set. The caller
// holds flushMtx.
func (s *syncController) prepareChunks(
	ctx context.Context,
	through uint64,
	orderBlocks bool,
	emit func(*preparedSyncChunk) error,
) (_ int, err error) {
	// Keep only one partial chunk alongside the current candidate page.
	maxBlocks := int(writer.DefaultMaxBlocksPerPack)
	pending := make([]dirtyCandidate, 0, maxBlocks)
	var pendingBytes int64

	// corrupt collects the dirty blocks this pass holds back. Unless a recheck
	// is due, the blocks the last pass found corrupt are held without a read.
	corrupt := make(map[string]struct{})
	var held, known map[string]struct{}
	var writes uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { held, writes = s.hold.blocks, s.writes })
	recheck := s.corruptRecheck.Swap(false)
	if !recheck {
		known = held
	}

	// A pass that stops early keeps what it found and every earlier hold,
	// and leaves a due recheck for the next pass.
	defer func() {
		if err == nil {
			return
		}
		if recheck {
			s.corruptRecheck.Store(true)
		}
		s.setCorruptKnown(corrupt, writes, true)
	}()
	flushPending := func() error {
		// Load the pending payloads and emit their packs.
		loaded, err := s.loadDirtyBlocks(ctx, pending, held, corrupt)
		if err != nil {
			return err
		}
		if err := s.prepareLoadedBlocks(loaded, emit); err != nil {
			return err
		}
		pending, pendingBytes = pending[:0], 0
		return nil
	}

	// Hold catalog membership fixed while sharing resident indexes across
	// pages. Trash packs do not count, since a reclaim pass retires them.
	view := s.lower.SnapshotManifest().WithoutTrash()

	// Batch duplicate checks after releasing each metadata transaction.
	for after := uint64(0); after < through; {
		page, err := s.scanDirtyPage(ctx, after, through)
		if err != nil {
			return 0, err
		}
		if len(page) == 0 {
			break
		}
		after = page[len(page)-1].sequence
		blocks, duplicates, err := s.filterDuplicateDirtyBlocks(ctx, view, page)
		if err != nil {
			return 0, errors.Wrap(err, "filtering duplicate dirty blocks")
		}
		if len(duplicates) != 0 {
			if err := s.cleanupDirtyCandidates(ctx, duplicates); err != nil {
				return 0, err
			}
		}
		if orderBlocks {
			blocks, err = s.orderDirtyBlocks(ctx, blocks)
			if err != nil {
				return 0, err
			}
		}

		// Page boundaries never force an extra undersized upload.
		for _, candidate := range blocks {
			size := dirtyCandidateChunkSize(candidate.size)
			if size > writer.DefaultMaxPackBytes {
				return 0, errors.Errorf("dirty block %s exceeds max pack chunk size", candidate.hash.MarshalString())
			}
			if len(known) != 0 {
				if key := candidate.hash.MarshalString(); hasKey(known, key) {
					corrupt[key] = struct{}{}
					continue
				}
			}
			if len(pending) != 0 && pendingBytes+size > syncFlushMaxPackBytes {
				if err := flushPending(); err != nil {
					return 0, err
				}
			}
			pending = append(pending, candidate)
			pendingBytes += size
			if len(pending) == maxBlocks {
				if err := flushPending(); err != nil {
					return 0, err
				}
			}
		}
	}
	if len(pending) != 0 {
		if err := flushPending(); err != nil {
			return 0, err
		}
	}

	// Keep and report the corrupt blocks the pass left queued.
	s.setCorruptKnown(corrupt, writes, false)
	return len(corrupt), nil
}

// setCorruptKnown records the corrupt dirty blocks the last flush held back.
// The count follows the markers: a marker cleared, as a duplicate or by a
// successful read, leaves the set at the next flush, and a flush with nothing
// pending clears it. writes is the write count the flush read with its copy
// of the held set.
//
// A partial flush stopped before reading every dirty block, on a push or
// commit failure or a cancel. Its set joins the current one rather than
// replacing it, since the blocks it did not reach keep their holds, and the
// held state is marked partial: the count is a lower bound. A block the flush
// read sound has already left the current set (loadDirtyBlocks), so the join
// does not count it. A partial flush that found nothing while nothing is held
// leaves the store not held; that is the one case the held state cannot know,
// since the blocks the flush did not read may be damaged.
//
// Invariants: nothing here deletes or acknowledges a block or a marker; a
// flush skips a block only while its hash is held; and a path that could
// leave a sound hash held sets corruptRecheck. A write of a held hash always
// makes a later flush read the block: MarkDirty removes the hash at once, but
// this replacement would put back a hash the flush read before that write
// landed. The flush cannot tell which hashes a write touched, so when any
// write landed since its copy it makes the next flush read every held block.
// The caller holds flushMtx.
func (s *syncController) setCorruptKnown(corrupt map[string]struct{}, writes uint64, partial bool) {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Join a partial flush's set to the current one.
		if partial && len(s.hold.blocks) != 0 {
			next := maps.Clone(s.hold.blocks)
			maps.Copy(next, corrupt)
			corrupt = next
		}
		if len(corrupt) != 0 && s.writes != writes {
			s.corruptRecheck.Store(true)
		}
		s.setHold(corrupt, partial)
	})
}

// releaseHold takes key out of the held set: a flush read its block sound.
func (s *syncController) releaseHold(key string) {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !hasKey(s.hold.blocks, key) {
			return
		}
		next := maps.Clone(s.hold.blocks)
		delete(next, key)
		s.setHold(next, s.hold.partial)
	})
}

// setHold replaces the held set with blocks, partial when the pass that found
// them stopped early, and shows the result in the sync status in the same
// step, so the status never shows a hold the set no longer has. It is the
// only writer of s.hold. An empty set is not held: it clears since and
// partial. The first hold sets since; later ones keep it. The caller holds
// bcast; the telemetry store takes only its own lock.
func (s *syncController) setHold(blocks map[string]struct{}, partial bool) {
	// Derive the next state: a non-empty set is held since the first hold.
	prev := s.hold
	next := holdState{}
	if len(blocks) != 0 {
		next = holdState{blocks: blocks, since: prev.since, partial: partial}
		if next.since.IsZero() {
			next.since = time.Now()
		}
	}
	s.hold = next

	// Show a changed count, partial or since in the sync status.
	if len(prev.blocks) == len(next.blocks) && prev.partial == next.partial && prev.since.Equal(next.since) {
		return
	}
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.setSyncTelemetryCorruptDirty(id, len(next.blocks), next.since, next.partial)
	})
}

// withoutMarks returns known without the hashes in marks. It returns known
// itself when no mark is in it, and a new map otherwise, since the map is
// replaced, never changed.
func withoutMarks(known map[string]struct{}, marks []block_store_writeback.Mark) map[string]struct{} {
	var out map[string]struct{}
	for _, m := range marks {
		key := m.Hash.MarshalString()
		if !hasKey(known, key) {
			continue
		}
		if out == nil {
			out = maps.Clone(known)
		}
		delete(out, key)
	}
	if out == nil {
		return known
	}
	return out
}

// held returns the held state of a flush that held count blocks back. It
// takes the count from the flush, not from the held set, so a write that
// clears a known entry meanwhile cannot turn a held flush into a success.
func (s *syncController) held(count int) *heldError {
	h := &heldError{count: count}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { h.since = s.hold.since })
	return h
}

// hasKey reports whether set holds key.
func hasKey(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// prepareLoadedBlocks packs loaded dirty blocks, halving a set whose pack
// exceeds the sync pack target, and passes each pack to emit.
func (s *syncController) prepareLoadedBlocks(blocks []dirtyBlock, emit func(*preparedSyncChunk) error) error {
	// Pack the blocks and halve a set whose pack is over the target.
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
	// Record the pack, then clear the markers of its blocks.
	if err := s.applyManifestDelta(ctx, []*packfile.PackfileEntry{chunk.entry}, nil, 0); err != nil {
		return errors.Wrap(err, "applying push delta")
	}
	flushed := make([]dirtyCandidate, len(chunk.blocks))
	for i, block := range chunk.blocks {
		flushed[i] = block.dirtyCandidate
	}
	return s.cleanupDirtyCandidates(ctx, flushed)
}

// prepareFlushChunk packs one bounded dirty-block chunk. Returns nil when
// blocks is empty.
func (s *syncController) prepareFlushChunk(blocks []dirtyBlock) (*preparedSyncChunk, error) {
	// Skip an empty chunk.
	if len(blocks) == 0 {
		return nil, nil
	}

	// Pack the dirty blocks in order.
	started := time.Now()
	packed := make([]packfile_store.PackBlock, len(blocks))
	for i, b := range blocks {
		packed[i] = packfile_store.PackBlock{Hash: b.hash, Block: b.stored}
	}
	chunk, err := s.preparePackBlocks(packed, nil)
	s.le.WithField("blocks", len(blocks)).
		WithField("duration", time.Since(started)).
		Debug("packed dirty blocks")
	if err != nil {
		return nil, err
	}
	chunk.blocks = blocks
	return chunk, nil
}

// pushPreparedChunk uploads one prepared packfile chunk.
func (s *syncController) pushPreparedChunk(ctx context.Context, chunk *preparedSyncChunk) error {
	// Push the pack, reporting its progress to telemetry.
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

	// Log and report the result.
	s.le.WithField("pack-id", entry.GetId()).
		WithField("blocks", entry.GetBlockCount()).
		WithField("bytes", len(chunk.packData)).
		WithField("duration", time.Since(started)).
		Debug("pushed packfile")
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.finishSyncTelemetryPush(id, pushBytes, err)
	})
	return errors.Wrap(err, "pushing packfile")
}

// flush collects dirty blocks, packs them, pushes to the server, and updates
// the manifest. It returns the count of corrupt dirty blocks it held back.
func (s *syncController) flush(ctx context.Context, orderBlocks bool) (int, error) {
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
		return 0, err
	}
	if state.GetCount() == 0 {
		s.setCorruptKnown(nil, 0, false)
		return 0, nil
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
		var dropped []string
		switch {
		case !page.GetMore():
			cursor = page.GetLatestSequence()
			if dropped = s.unlistedPacks(listed); len(dropped) != 0 {
				events = append(events, &packfile.PackReplacementEvent{ReplacedPackIds: dropped})
			}
		case listed == nil:
			cursor = next
		}

		if err := s.applyManifestDelta(ctx, page.GetEntries(), events, cursor); err != nil {
			return errors.Wrap(err, "applying pull delta")
		}

		// A pack marked as trash or dropped from a restarted listing no
		// longer holds linked chunks for the lane.
		if s.linked != nil && (len(dropped) != 0 || slices.ContainsFunc(page.GetEntries(), (*packfile.PackfileEntry).IsTrash)) {
			s.linked.requestReconcile()
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
