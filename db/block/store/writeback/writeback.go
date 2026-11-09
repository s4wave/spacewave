// Package block_store_writeback keeps every block write in a local store and
// uploads it to a remote store in the background.
//
// A write is acknowledged once the local store holds the block and a durable
// marker records it. Atomic local stores commit the marker in the same
// transaction (see kvtx.BlockMarker). Other stores, including RPC proxies,
// persist the block before its marker; a marker failure leaves an unacknowledged
// block that a repeated write queues again. Upload drains the markers to the
// remote store
// and clears each marker only after the remote store holds its block, so
// writes made while the remote is unreachable upload after it returns, across
// restarts. Backfill queues the blocks stored before a remote was chosen.
//
// A marker waits a settle age before it uploads, so a block the garbage
// collector sweeps meanwhile never reaches the remote: the upload finds the
// block gone and drops its marker.
package block_store_writeback

import (
	"bytes"
	"context"
	"math/rand/v2"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// backfillKey names the remote whose backfill completed.
const backfillKey = "backfill"

// formerPrefixes match the keys of the marker layout before the queue,
// dropped at open so the next backfill queues their blocks again, and those
// of an earlier count of the queue.
var formerPrefixes = []string{"pending/", "target", "queue-pending"}

// backfillBatchSize bounds the blocks one backfill marker transaction queues.
const backfillBatchSize = 256

// errScanDone stops a queue scan.
var errScanDone = errors.New("queue scan done")

// Store is a local block store whose writes queue for upload.
type Store struct {
	*MarkingStore

	// local holds every block and serves the uploads.
	local block.StoreOps
	// values holds the local block values below any ownership tracking, so
	// eviction and refill change values alone.
	values block.StoreOps
	// markers holds one marker per block awaiting upload, in the key-value
	// store that holds the blocks.
	markers kvtx.Store
	// markersID is the ID of the object store that markers views.
	markersID string
	// mtx orders upload clears, backfill queueing and enablement changes with
	// their projection into status. Block writes mark without it, inside their
	// own transactions.
	mtx csync.Mutex
	// evictMtx runs one eviction pass at a time, and disabling uploads waits
	// for the pass to end.
	evictMtx csync.Mutex
	// evictFrom is the eviction queue key the next eviction batch walks
	// from, guarded by evictMtx: where the last committed batch stopped,
	// with every entry before it walked. Entries before it are deleted, and
	// their tombstones stay in the store until it compacts them, so a walk
	// from the start of the queue would step over them again on every batch.
	// Every entry queues in a write transaction at the epoch it reads there,
	// and a pass advances the epoch before it walks, so a later entry sorts
	// at or past it. Nil walks from the start, as after an open.
	evictFrom []byte
	// evictLimit is the block limit of the next eviction batch, guarded by
	// evictMtx, as nextEvictLimit sets it; evictBatchStartBlocks on open.
	evictLimit int
	// bcast guards status, and wakes Upload and status watchers.
	bcast broadcast.Broadcast
	// status projects the committed markers and the upload policy.
	status Status
	// gen counts the drops of the queue, guarded by bcast, which a disable
	// makes before and after its transaction. A transaction that changed the
	// queue or its count projects the change into status only if no drop
	// followed its start: the drop zeroed the status, and the count with it.
	gen uint64
	// floor is a key at or before every queue entry a write transaction
	// committed or holds in flight, or nil for the queue's start; guarded by
	// bcast. Queue scans start from it. A transaction lowers it before it
	// commits, and clear raises it to the queue's new head.
	floor []byte
	// lowered counts the transactions that lowered floor, guarded by bcast,
	// so clear raises it only when none lowered it since clear read it.
	lowered uint64
	// owner names this open as the owner of the walk that starts the
	// queue's kept count, as countKey describes.
	owner uint64
	// netQueuedBytes is the bytes queued less the bytes cleared since the
	// open, guarded by bcast: the queue's growth, which the walk that starts
	// the count does not move, so a drain's regrow measures new writes alone.
	netQueuedBytes int64
	// queued counts the projected transactions that queued blocks, guarded
	// by bcast, so an upload that read an empty head wakes on the next one
	// whatever Oldest says.
	queued uint64
	// locate is the switch that skips uploading the blocks the remote already
	// holds, nil while none was set.
	locate atomic.Pointer[atomic.Bool]
	// reads bounds the upload's local reads in flight by the disk's load.
	reads *readGate
	// fills queues blocks read from the remote for their local writes.
	fills fillQueue
}

// GetStoreParts returns the local store that holds every block.
func (s *Store) GetStoreParts() []block.StorePart {
	return []block.StorePart{{Label: "local", Store: s.local}}
}

// NewStore builds a Store over local, counting the markers left by a previous
// run. Queued blocks upload at once until SetPolicy sets a settle age. values
// holds the same blocks as local without its ownership tracking.
//
// markers is the object store with ID markersID in the key-value store that
// holds the blocks of local. Atomic local stores must support kvtx.BlockMarker
// in their writing transactions. Other stores queue markers after their block
// writes succeed, before acknowledging the write.
//
// When enabled is false, writes are not queued until SetEnabled enables them.
func NewStore(
	ctx context.Context,
	local, values block.StoreOps,
	markers kvtx.Store,
	markersID string,
	enabled bool,
) (*Store, error) {
	return newStore(ctx, local, values, markers, markersID, enabled, false)
}

// NewStoreWithPacks builds a Store as NewStore does over packed, which reads
// its packahead Log before its local store. While packed diverts, its new
// values go to the Log: they get no marker, since a finalized pack is their
// upload queue entry, uploaded by a packahead Drain, and the Volume holds only
// their child lists. The blocks local held before, and the cache fills, keep
// the marker path, as every write does while packed does not divert.
func NewStoreWithPacks(
	ctx context.Context,
	packed *PackedStore,
	values block.StoreOps,
	markers kvtx.Store,
	markersID string,
	enabled bool,
) (*Store, error) {
	return newStore(ctx, packed, values, markers, markersID, enabled, packed.divert)
}

// newStore builds the Store over writes, which marks only cache fills when
// divert is set.
func newStore(
	ctx context.Context,
	writes, values block.StoreOps,
	markers kvtx.Store,
	markersID string,
	enabled bool,
	divert bool,
) (*Store, error) {
	// Connect local block writes to the durable upload markers. Diverted
	// values go to their packs, which need none, so a diverting store marks
	// only its cache fills.
	s := &Store{local: writes, values: values, markers: markers, markersID: markersID, evictLimit: evictBatchStartBlocks, reads: newReadGate(), owner: rand.Uint64()} //nolint:gosec // the owner tells opens apart and guards nothing.
	if supportsAtomicPublication(writes) {
		s.MarkingStore = NewTxMarkingStore(writes, s)
	} else {
		s.MarkingStore = NewMarkingStore(writes, s.queue)
	}
	if divert {
		s.MarkingStore = NewFillMarkingStore(s.MarkingStore)
	}

	// Restore the queue's kept count, its head, the eviction queue size and
	// the completed backfill target.
	// A read-only open reads the same state and writes none of it.
	var (
		count          queueCount
		evictableBytes int64
		target         []byte
		oldest         time.Time
	)
	readOnly := readOnlyOpen.Load()
	openTx := s.writeTx
	if readOnly {
		openTx = func(ctx context.Context) (kvtx.Tx, error) { return s.markers.NewTransaction(ctx, false) }
	}
	err := kvtx.RunTransaction(ctx, !readOnly, openTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Drop the former marker layout before reading the queue.
		for _, prefix := range formerPrefixes {
			if readOnly {
				break
			}
			if err := deletePrefix(ctx, tx, prefix); err != nil {
				return err
			}
		}

		// Read the completed backfill target and the eviction queue size.
		var err error
		target, _, err = tx.Get(ctx, []byte(backfillKey))
		if err != nil {
			return err
		}
		evictableBytes, err = readEvictableBytes(ctx, tx)
		if err != nil {
			return err
		}

		// Read the queue's head.
		oldest = time.Time{}
		err = scanQueue(ctx, tx, nil, func(item queueItem) error {
			at, err := item.at()
			if err != nil {
				return err
			}
			oldest = at
			return errScanDone
		})
		if err != nil && !errors.Is(err, errScanDone) {
			return err
		}

		// Take the kept count, or start the walk that counts the queue: a
		// store that never kept one, or the switch to recount, walks from the
		// queue's start to just past its last key, and an empty queue needs
		// no walk. A walk left running takes this open as its owner.
		var found bool
		count, found, err = readQueueCount(ctx, tx)
		if err != nil {
			return err
		}
		if readOnly {
			count.done = true
			return nil
		}
		switch {
		case found && count.done && !recountAtOpen.Load():
			return nil
		case found && !recountAtOpen.Load():
		case oldest.IsZero():
			count = queueCount{done: true}
		default:
			last, err := queueLast(ctx, tx)
			if err != nil {
				return err
			}
			count = queueCount{end: append(last, 0)}
		}
		count.owner = s.owner
		return writeQueueCount(ctx, tx, count)
	})
	if err != nil {
		return nil, errors.Wrap(err, "read pending uploads")
	}

	// Initialize upload status from the committed marker records, and walk
	// the rest of the queue in the background.
	s.status = Status{
		Enabled:        enabled,
		Pending:        count.n,
		PendingBytes:   count.bytes,
		Counting:       !count.done,
		Oldest:         oldest,
		Target:         string(target),
		EvictableBytes: evictableBytes,
	}
	if !count.done {
		go s.walk(ctx)
	}
	return s, nil
}

// walkSliceLimit bounds the queue entries one slice of the walk counts, and
// so how long its transaction holds the store's writer.
const walkSliceLimit = 1000

// walkRetry is how long the walk waits to try a failed slice again.
const walkRetry = time.Second

// walk counts the queue into its kept count from the cursor on, a slice at a
// time, until it reaches the queue's end, as countKey describes. Nothing waits
// for it: clears, the upload, the drain and backfills keep the count exact
// for covered keys meanwhile, and the walk counts the rest as it reaches
// them. The status holds the count, short of the queue and Counting until
// the walk ends. A restart resumes from the cursor the last slice committed,
// and once done the walk never runs again.
//
// Each slice counts in a background write transaction, which yields to
// foreground writers, after a read that holds no lock warms its pages, so
// the writer is held for a read of cached pages. After each slice the walk
// waits as long as the slice took, so it takes at most half the store's
// time. A failed slice is tried again: until the walk ends, the count stays
// short.
func (s *Store) walk(ctx context.Context) {
	for {
		start := time.Now()
		done, err := s.walkSlice(ctx)
		if done || ctx.Err() != nil {
			return
		}
		wait := time.Since(start)
		if err != nil {
			wait = walkRetry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// walkSlice counts the next slice of the queue past the walk's cursor and
// moves the cursor over it in one transaction, or ends the walk at the
// queue's end, reporting true once it has ended.
func (s *Store) walkSlice(ctx context.Context) (bool, error) {
	// Warm the slice's pages, then count it behind every foreground writer.
	gen, _ := s.getEnabledGen()
	_ = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		_, _, _, err := s.countSlice(ctx, tx)
		return err
	})
	var delta countDelta
	var done bool
	err := kvtx.RunTransaction(kvtx.WithBackgroundWriteContext(ctx), true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Count the slice and keep the count moved past it.
		count, d, moved, err := s.countSlice(ctx, tx)
		delta, done = d, count.done
		if err != nil || !moved {
			return err
		}
		return writeQueueCount(ctx, tx, count)
	})
	if err != nil {
		return false, err
	}

	// Project the slice into the status, unless a drop of the queue followed
	// its start.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Add the slice, then wake watchers. Only the end of the walk moves
		// Counting, since a clear that empties the queue may end it first.
		if s.gen != gen {
			return
		}
		s.status.Pending += delta.n
		s.status.PendingBytes += delta.bytes
		if done {
			s.status.Counting = false
		}
		broadcast()
	})
	return done, nil
}

// countSlice reads the kept count in tx and counts the next slice of the
// queue past its cursor into it, returning the count moved past the slice,
// or done at the walk's end, the change, and whether it moved the count. A
// walk that is done, or that a later open owns, reads as done and unmoved.
func (s *Store) countSlice(ctx context.Context, tx kvtx.Tx) (queueCount, countDelta, bool, error) {
	// Read the count. A done walk counts nothing, and neither does a slice
	// of a store a later open took the walk from, which ends its walk.
	count, found, err := readQueueCount(ctx, tx)
	if err != nil || !found || count.done || count.owner != s.owner {
		return queueCount{done: true}, countDelta{}, false, err
	}

	// Count the entries past the cursor, from the floor when it is later:
	// no entry sorts before it.
	from := count.cursor
	floor, _ := s.readFloor()
	if floor != nil && (from == nil || bytes.Compare(floor, from) > 0) {
		from = floor
	}
	var delta countDelta
	var last []byte
	ended := true
	err = scanQueue(ctx, tx, from, func(item queueItem) error {
		// Skip the cursor itself, and stop at the end or past the slice.
		if count.cursor != nil && bytes.Compare(item.key, count.cursor) <= 0 {
			return nil
		}
		if bytes.Compare(item.key, count.end) >= 0 {
			return errScanDone
		}
		if delta.n == walkSliceLimit {
			ended = false
			return errScanDone
		}

		// Count the entry.
		delta.n++
		delta.bytes += item.size()
		last = append(last[:0], item.key...)
		return nil
	})
	if err != nil && !errors.Is(err, errScanDone) {
		return count, countDelta{}, false, err
	}

	// Move the cursor past the slice, or end the walk.
	count.n += delta.n
	count.bytes += delta.bytes
	if ended {
		count = queueCount{n: count.n, bytes: count.bytes, done: true}
	} else {
		count.cursor = last
	}
	return count, delta, true, nil
}

// SetPolicy sets when queued blocks upload and the resident budget, waking
// Upload and budget watchers to apply it.
func (s *Store) SetPolicy(policy Policy) {
	s.reads.setPolicy(policy.MaxUploadReads, policy.UploadReadSlow)
	s.fills.setLimit(policy.FillQueueBytes)
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.status.Policy != policy {
			s.status.Policy = policy
			broadcast()
		}
	})
}

// GetStatus returns the upload status and a channel closed when it changes.
func (s *Store) GetStatus() (Status, <-chan struct{}) {
	// Copy the status and its change channel.
	var status Status
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		status, changed = s.status, getWaitCh()
	})

	// Add the read bound, which moves without a broadcast.
	status.UploadReads = s.reads.getLimit()
	return status, changed
}

// SetEnabled starts or stops queueing writes for upload.
//
// Enabling returns once every write transaction that began while queueing was
// off has ended, so a Backfill that follows lists their blocks. Disabling
// drops the queued markers, the eviction queue and the backfill target: the
// remote store no longer needs the markers, and the local store must keep the
// blocks it holds.
func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	// Serialize upload enablement with marker mutations.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Drop pending markers and the backfill target after any eviction pass,
	// whose commits would count entries dropped here.
	if !enabled {
		releaseEvict, err := s.evictMtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer releaseEvict()
	}

	// Publish upload enablement first, so a write that starts after this point
	// queues or skips by the new setting, and after a disable one in flight
	// projects nothing.
	drop := func() {
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			s.status.Enabled = enabled
			if !enabled {
				s.gen++
				s.status.Pending, s.status.PendingBytes = 0, 0
				s.status.Counting = false
				s.status.EvictableBytes = 0
				s.status.Oldest = time.Time{}
				s.status.Err, s.status.Target = nil, ""
			}
			broadcast()
		})
	}
	drop()

	// A write transaction ahead of this one in the store's order either marked
	// by the old setting or ends before the write below returns. Dropping the
	// markers also removes those a disabled-to-enabled race left behind.
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Drop the queue, its count and the eviction queue.
		if enabled {
			return nil
		}
		for _, prefix := range []string{markedPrefix, queuePrefix, evictPrefix} {
			if err := deletePrefix(ctx, tx, prefix); err != nil {
				return err
			}
		}
		if err := writeQueueCount(ctx, tx, queueCount{done: true}); err != nil {
			return err
		}
		if err := tx.Delete(ctx, []byte(evictableBytesKey)); err != nil {
			return err
		}
		return tx.Delete(ctx, []byte(backfillKey))
	})
	if err != nil {
		return errors.Wrap(err, "change upload enablement")
	}

	// Drop again after the commit, so a walk slice that committed ahead of
	// it and projects late projects nothing.
	drop()
	return nil
}

// Backfill queues every block list names for upload to target, unless a
// backfill for target already completed. Call it after enabling uploads for
// target, so the backfill and new writes together cover every block.
//
// A block the local store does not hold is skipped. The completed target is
// recorded durably, so an interrupted backfill runs again on the next call.
func (s *Store) Backfill(
	ctx context.Context,
	target string,
	list func(ctx context.Context) ([]*block.BlockRef, error),
) error {
	// Skip backfill when the selected remote target is already complete.
	status, _ := s.GetStatus()
	if status.Target == target {
		return nil
	}

	// Queue existing local blocks for the selected upload target.
	refs, err := list(ctx)
	if err != nil {
		return errors.Wrap(err, "list blocks to upload")
	}
	for batch := range slices.Chunk(refs, backfillBatchSize) {
		marks := make([]Mark, 0, len(batch))
		for _, ref := range batch {
			stat, err := s.local.StatBlock(ctx, ref)
			if err != nil {
				return errors.Wrap(err, "stat block to upload")
			}
			if stat != nil {
				marks = append(marks, Mark{Hash: ref.GetHash(), Size: max(stat.Size, 0)})
			}
		}
		if err := s.queue(ctx, marks); err != nil {
			return err
		}
	}

	// Record and publish the completed upload backfill target.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(backfillKey), []byte(target))
	})
	if err != nil {
		return errors.Wrap(err, "record upload backfill")
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Target = target
		broadcast()
	})
	return nil
}

// GetUploadReads returns how many local reads the upload currently keeps in
// flight at most.
func (s *Store) GetUploadReads() int {
	return s.reads.getLimit()
}

// UploadedBefore reports whether the remote holds every block written before
// at. It reads the queue's first entry from the committed markers rather than
// the status, which follows a commit, so a head read at at can be named durable
// the moment this returns true. A disabled store uploads nothing and reports
// false.
func (s *Store) UploadedBefore(ctx context.Context, at time.Time) (bool, error) {
	// A store that does not queue proves nothing about its blocks.
	if status, _ := s.GetStatus(); !status.Enabled {
		return false, nil
	}

	// Read the floor after the transaction opens: a clear that raised it past
	// an entry this view holds removed that entry after uploading it.
	tx, err := s.markers.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	floor, _ := s.readFloor()

	// Compare the first entry, which is the oldest block awaiting upload.
	uploaded := true
	err = scanQueue(ctx, tx, floor, func(item queueItem) error {
		first, err := item.at()
		if err != nil {
			return err
		}
		uploaded = !first.Before(at)
		return errScanDone
	})
	if err != nil && !errors.Is(err, errScanDone) {
		return false, err
	}
	return uploaded, nil
}

// MarkerStoreID returns the ID of the object store that holds the markers.
func (s *Store) MarkerStoreID() string {
	return s.markersID
}

// MarkBlocks queues the blocks a write stores, in the transaction that stores
// them. The queue status follows once the transaction commits.
//
// A disabled store queues nothing. SetEnabled orders itself against the
// transactions in flight, so a block stored while disabled is listed by the
// Backfill that follows enabling.
func (s *Store) MarkBlocks(ctx context.Context, tx kvtx.Tx, blocks []kvtx.BlockMark) (func(), error) {
	// Skip upload markers while queueing is disabled.
	gen, enabled := s.getEnabledGen()
	if !enabled {
		return nil, nil
	}

	// Queue the blocks at one write time.
	now := time.Now()
	marks := make([]Mark, len(blocks))
	for i, b := range blocks {
		marks[i] = Mark{Hash: b.Hash, Size: b.Size, At: now}
	}
	delta, err := queueMarks(ctx, tx, marks)
	if err != nil {
		return nil, errors.Wrap(err, "queue block upload")
	}
	if delta.added == 0 {
		return nil, nil
	}
	s.lowerFloor(delta.oldest)
	return func() { s.applyQueued(gen, delta) }, nil
}

// queue queues blocks for upload in one transaction of their own, as a
// backfill does for blocks that are stored already.
//
// A disabled store skips the lock. The check repeats under the lock so a call
// racing SetEnabled(false) cannot queue after the queue is dropped.
func (s *Store) queue(ctx context.Context, marks []Mark) error {
	// Skip upload markers while queueing is disabled.
	if !s.isEnabled() {
		return nil
	}

	// Serialize marker writes and recheck upload enablement.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	gen, enabled := s.getEnabledGen()
	if !enabled {
		return nil
	}

	// Queue the markers in one write transaction.
	var delta queueDelta
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		delta, err = queueMarks(ctx, tx, marks)
		if err == nil && delta.added != 0 {
			s.lowerFloor(delta.oldest)
		}
		return err
	})
	if err != nil {
		return errors.Wrap(err, "queue block upload")
	}
	s.applyQueued(gen, delta)
	return nil
}

// lowerFloor lowers the queue floor to the entries written at oldest, inside
// the transaction that queues them, so no scan that starts after the commit
// passes them.
func (s *Store) lowerFloor(oldest time.Time) {
	key := queueFloor(oldest)
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.floor != nil && bytes.Compare(key, s.floor) < 0 {
			s.floor = key
		}
		s.lowered++
	})
}

// readFloor returns the queue floor and the count of transactions that
// lowered it.
func (s *Store) readFloor() ([]byte, uint64) {
	var floor []byte
	var lowered uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		floor, lowered = s.floor, s.lowered
	})
	return floor, lowered
}

// applyQueued projects blocks queued by a committed transaction into the
// status and wakes Upload, unless the queue was dropped since the transaction
// marked them: the drop zeroed the status. The status counts only the entries
// the kept count covers.
func (s *Store) applyQueued(gen uint64, delta queueDelta) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Ignore a transaction that predates the latest enable or disable.
		if s.gen != gen || !s.status.Enabled {
			return
		}
		if s.status.Oldest.IsZero() || delta.oldest.Before(s.status.Oldest) {
			s.status.Oldest = delta.oldest
		}
		s.status.Pending += delta.counted.n
		s.status.PendingBytes += delta.counted.bytes
		s.netQueuedBytes += delta.addedBytes
		s.queued++
		broadcast()
	})
}

// clear removes the markers of uploaded blocks and queues them for eviction at
// the current epoch.
//
// A dropped block written again since the batch read it keeps its marker: the
// write changed the queue entry's value, and the block is on the local store
// alone.
//
// The batch clears in transactions of at most ClearBatch.MaxBlocks entries,
// each holding the writer for about ClearBatch.Hold at most, as SetClearBatch
// sets them: a change of the hold applies from the next transaction, and one
// of the block bound from the next prefetch window. A transaction the store
// reports busy is retried in place after a backoff, as retryBusy describes,
// since the blocks are already on the remote. Another failure leaves the entries after the last
// commit queued, so they upload again, which the remote keeps as another copy.
func (s *Store) clear(ctx context.Context, batch []pendingBlock) error {
	for len(batch) != 0 {
		// Read what the clears read before taking any lock, so the reads
		// under the writer find their pages cached.
		window := batch[:min(len(batch), loadClearBatch().MaxBlocks)]
		batch = batch[len(window):]
		s.prefetchClear(ctx, window)
		for len(window) != 0 {
			var n int
			err := retryBusy(ctx, func() error {
				var err error
				n, err = s.clearSome(ctx, window)
				return err
			})
			if err != nil {
				return err
			}
			window = window[n:]
		}
	}
	return nil
}

// busyRetries bounds how often retryBusy retries a transaction the store
// reported busy, and busyWait is its first wait, doubling to busyMaxWait: about
// 80 seconds in all.
const (
	busyRetries = 20
	busyWait    = 250 * time.Millisecond
	busyMaxWait = 5 * time.Second
)

// retryBusy runs fn, a transaction that commits whole or not at all, and runs
// it again after a backoff while the store reports itself busy, up to
// busyRetries times. Each attempt is its own transaction, so a retry holds the
// writer no longer than the first attempt did. Returns fn's last error, or the
// context's cause if it ends during a wait.
func retryBusy(ctx context.Context, fn func() error) error {
	wait := busyWait
	for busy := 0; ; busy++ {
		err := fn()
		if err == nil || busy >= busyRetries || !IsBusyError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(wait):
		}
		wait = min(2*wait, busyMaxWait)
	}
}

// IsBusyError reports whether err is a store's report that another writer
// holds its lock, SQLite's "database is locked", which a later attempt may
// pass.
func IsBusyError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database is locked")
}

// clearSome clears the batch's leading entries in one transaction, as many as
// the hold in force admits and at least one, and returns how many it cleared.
func (s *Store) clearSome(ctx context.Context, batch []pendingBlock) (int, error) {
	// Clear behind every foreground writer: the upload is background work,
	// and a player's write must not queue behind its marker clears. The
	// writers that block on mtx while a clear waits are a backfill, a
	// change of enabled and the marking store of a store that cannot mark
	// in its own transaction; a store that marks in the write's transaction
	// takes no mtx.
	ctx = kvtx.WithBackgroundWriteContext(ctx)

	// Serialize uploaded marker removal with queue status updates.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return 0, err
	}
	defer release()

	// Remove uploaded block markers, count their cleared bytes, and read the
	// write time of the queue's new head.
	var taken, cleared, uploaded int
	var clearedBytes, uploadedBytes, queuedBytes int64
	var oldest time.Time
	var head []byte
	var lowered uint64

	// Read the hold in force for this transaction.
	hold := loadClearBatch().Hold

	// Keep the change to the queue's count, and whether it ended the walk.
	var counted countDelta
	var ended bool
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// The writer is held from here.
		held := time.Now()

		// Delete the markers still queued, counting what they held, and queue
		// the uploaded blocks for eviction.
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		count, kept, err := readQueueCount(ctx, tx)
		if err != nil {
			return err
		}
		taken, cleared, uploaded, clearedBytes, uploadedBytes, queuedBytes = 0, 0, 0, 0, 0, 0
		counted, ended = countDelta{}, false
		for _, pending := range batch {
			// Commit once the writer was held for the hold, leaving the rest
			// to the next transaction.
			if taken != 0 && time.Since(held) >= hold {
				break
			}
			taken++
			value, found, err := tx.Get(ctx, pending.queueKey)
			if err != nil {
				return err
			}
			if !found || (pending.dropped && !bytes.Equal(value, pending.value)) {
				continue
			}
			if err := tx.Delete(ctx, pending.queueKey); err != nil {
				return err
			}
			if err := tx.Delete(ctx, pending.markedKey); err != nil {
				return err
			}
			if !pending.dropped {
				added, err := queueEvict(ctx, tx, epoch, pending.ref.GetHash(), pending.size)
				if err != nil {
					return err
				}
				queuedBytes += added
				uploaded++
				uploadedBytes += pending.size
			}
			cleared++
			clearedBytes += pending.size
			if kept {
				counted = counted.plus(count.remove(pending.queueKey, queueItem{value: value}.size()))
			}
		}
		if err := addEvictableBytes(ctx, tx, queuedBytes); err != nil {
			return err
		}

		// Read the new queue head's write time from the floor. The store
		// orders write transactions, so every transaction that lowered the
		// floor before this read committed or discarded already.
		var floor []byte
		floor, lowered = s.readFloor()
		oldest, head = time.Time{}, []byte(queueEnd)
		err = scanQueue(ctx, tx, floor, func(item queueItem) error {
			at, err := item.at()
			if err != nil {
				return err
			}
			oldest, head = at, queueFloor(at)
			return errScanDone
		})
		if err != nil && !errors.Is(err, errScanDone) {
			return err
		}

		// Keep the count in the same transaction. An empty queue counts
		// zero, which the walk needs no longer, whatever a writer that kept
		// no count left it at.
		if !kept {
			return nil
		}
		if oldest.IsZero() && (count.n != 0 || count.bytes != 0 || !count.done) {
			counted = counted.plus(countDelta{n: -count.n, bytes: -count.bytes})
			ended, count = !count.done, queueCount{done: true}
		} else if counted == (countDelta{}) {
			return nil
		}
		return writeQueueCount(ctx, tx, count)
	})
	if err != nil {
		return 0, errors.Wrap(err, "clear uploaded blocks")
	}

	// Publish the remaining upload queue and clear its last failure.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Apply the change to the kept count, which covers the entries the
		// status holds.
		s.status.Pending += counted.n
		s.status.PendingBytes += counted.bytes
		if ended {
			s.status.Counting = false
		}
		s.netQueuedBytes -= clearedBytes
		s.status.Uploaded += int64(uploaded)
		s.status.UploadedBytes += uploadedBytes
		s.status.EvictableBytes += queuedBytes

		// Take the new head. Oldest is zero only for an empty queue, since
		// the upload sleeps on it: when the clear left none but a writer
		// lowered the floor since this clear read it, that writer's entries
		// may have committed after the read, so Oldest stays as it was, or
		// as the writer's projection sets it.
		if !oldest.IsZero() || s.lowered == lowered {
			s.status.Oldest = oldest
		}

		// Retain a floor lowered by a writer since this clear read it.
		if s.lowered == lowered || (s.floor != nil && bytes.Compare(head, s.floor) < 0) {
			s.floor = head
		}
		broadcast()
	})
	return taken, nil
}

// prefetchClear reads the keys clear reads, in a read transaction that holds
// no lock. It only warms the store's cache, so a failure is ignored and leaves
// clear to read the pages itself.
func (s *Store) prefetchClear(ctx context.Context, batch []pendingBlock) {
	_ = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the epoch and each pending block's queue, marked and evict
		// keys: the leaves clear reads, deletes and inserts into.
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		for _, pending := range batch {
			if _, _, err := tx.Get(ctx, pending.queueKey); err != nil {
				return err
			}
			if _, err := tx.Exists(ctx, pending.markedKey); err != nil {
				return err
			}
			if _, err := tx.Exists(ctx, evictKey(epoch, pending.ref.GetHash())); err != nil {
				return err
			}
		}

		// Read the resident count and the head of the queue.
		if _, err := readEvictableBytes(ctx, tx); err != nil {
			return err
		}
		floor, _ := s.readFloor()
		return scanQueue(ctx, tx, floor, func(queueItem) error { return errScanDone })
	})
}

// SetError records an upload failure in the status, such as a remote store
// that cannot be opened. The next successful batch clears it.
func (s *Store) SetError(err error) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Err = err
		broadcast()
	})
}

// isEnabled reports whether writes are queued for upload.
func (s *Store) isEnabled() bool {
	var enabled bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		enabled = s.status.Enabled
	})
	return enabled
}

// getEnabledGen reports whether writes are queued for upload, and the count of
// enablement changes that setting follows.
func (s *Store) getEnabledGen() (uint64, bool) {
	var gen uint64
	var enabled bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		gen, enabled = s.gen, s.status.Enabled
	})
	return gen, enabled
}

// readTx opens a read transaction on the marker store.
func (s *Store) readTx(ctx context.Context) (kvtx.Tx, error) {
	return s.markers.NewTransaction(ctx, false)
}

// writeTx opens a write transaction on the marker store.
func (s *Store) writeTx(ctx context.Context) (kvtx.Tx, error) {
	return s.markers.NewTransaction(ctx, true)
}

// _ is a type assertion
var (
	_ block.StoreOps   = (*Store)(nil)
	_ block.StoreParts = (*Store)(nil)
	_ kvtx.BlockMarker = (*Store)(nil)
)
