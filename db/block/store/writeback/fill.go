package block_store_writeback

import (
	"context"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sync/errgroup"
)

// defaultFillQueueBytes bounds the block bytes awaiting the drain when the
// policy sets no bound. A fill past it is dropped: the block stays readable
// from the remote, and the next read of it fills it again.
const defaultFillQueueBytes = 1 << 30

// fillBatchBlocks bounds the fills one drain step commits, so a writer queued
// behind the step waits for a bounded write.
const fillBatchBlocks = 256

// fillBatchBytes bounds the block bytes one drain step commits.
const fillBatchBytes = 8 << 20

// fill is a block read from a remote, awaiting its local write.
type fill struct {
	// ref is the block's reference.
	ref *block.BlockRef
	// stored is the block with its refs.
	stored *block.StoredBlock
	// refill reports a block whose child list the volume keeps, whose value
	// alone was evicted; any other block is a cache fill.
	refill bool
}

// fillQueue holds the fills no drain step has taken.
type fillQueue struct {
	mu sync.Mutex
	// pending are the queued fills, oldest first.
	pending []fill
	// bytes is the summed block size of pending.
	bytes int
	// limit bounds bytes; zero uses defaultFillQueueBytes.
	limit int64
	// queued holds, by hash, each block pending or being written, so a block
	// read again before its write lands is not queued twice.
	queued map[string]*queuedFill
	// draining records that a drain goroutine runs.
	draining bool
}

// queuedFill is the queue state of a block pending or being written. A fill
// with asserted refs that arrives for a block queued with refs it did not
// assert replaces the queued one, since the volume may drop the fill whose
// refs it does not trust: in place while the fill is pending, and after the
// write ends when a drain step took it.
type queuedFill struct {
	// asserted records that the queued fill asserts its refs.
	asserted bool
	// taken records that a drain step took the fill.
	taken bool
	// upgrade replaces the pending fill when the step takes it.
	upgrade *fill
	// again queues once the taken fill's write ends.
	again *fill
}

// setLimit sets the byte bound of the queue; zero uses defaultFillQueueBytes.
func (q *fillQueue) setLimit(limit int64) {
	q.mu.Lock()
	q.limit = max(limit, 0)
	q.mu.Unlock()
}

// fillKey keys a fill's block in queued.
func fillKey(ref *block.BlockRef) string {
	return string(ref.GetHash().GetHash())
}

// Refill queues a block read back from the remote store for its local value
// and its eviction queue entry. It changes no holder, so call it only for a
// block whose child list the volume still keeps. It returns before the write, and
// drops the block when the queue is full, as defaultFillQueueBytes describes.
func (s *Store) Refill(ref *block.BlockRef, stored *block.StoredBlock) {
	s.queueFill(fill{ref: ref, stored: stored, refill: true})
}

// CacheFill queues a remotely read block to be stored with its child refs but
// no ownership, then counted and queued for eviction. It creates no upload
// marker; eviction must verify the placement's remote holds the value before
// dropping it, including when this read came from a peer. It returns before
// the write, and drops the block when the queue is full.
func (s *Store) CacheFill(ref *block.BlockRef, stored *block.StoredBlock) {
	s.queueFill(fill{ref: ref, stored: stored})
}

// queueFill queues f, starting a drain when none runs.
func (s *Store) queueFill(f fill) {
	// Upgrade a queued block to asserted refs, or skip it.
	q := &s.fills
	q.mu.Lock()
	if queued, ok := q.queued[fillKey(f.ref)]; ok {
		if f.stored.RefsSource.Asserted() && !queued.asserted {
			if queued.taken {
				queued.again = &f
			} else {
				queued.upgrade = &f
			}
			queued.asserted = true
		}
		q.mu.Unlock()
		return
	}

	// Queue it, or drop a fill past the byte bound.
	if !q.pushLocked(f) {
		q.mu.Unlock()
		s.addFillStatus(0, 0, 1)
		return
	}
	drain := !q.draining
	q.draining = true
	q.mu.Unlock()

	// Start the drain this fill found idle.
	if drain {
		go s.drainFills()
	}
}

// pushLocked appends f to the pending fills, and reports false when it would
// pass the byte bound. The caller holds mu.
func (q *fillQueue) pushLocked(f fill) bool {
	// Refuse a fill past the byte bound.
	limit := q.limit
	if limit == 0 {
		limit = defaultFillQueueBytes
	}
	if int64(q.bytes+len(f.stored.Data)) > limit {
		return false
	}

	// Queue it under its hash.
	if q.queued == nil {
		q.queued = make(map[string]*queuedFill)
	}
	q.queued[fillKey(f.ref)] = &queuedFill{asserted: f.stored.RefsSource.Asserted()}
	q.pending = append(q.pending, f)
	q.bytes += len(f.stored.Data)
	return true
}

// drainFills writes the queued fills in bounded steps until none are left.
// Fills are disposable copies of blocks the remote holds, so their commits
// are ordered rather than flushed, and use background admission.
// A failed step drops its fills.
func (s *Store) drainFills() {
	// Use background admission and ordered durability for remote copies.
	ctx := block.WithBackground(kvtx.WithOrderedCommitContext(context.Background()))
	q := &s.fills
	for {
		// Take the next step, or end the drain on an empty queue.
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.draining = false
			q.mu.Unlock()
			return
		}
		n, size := 0, 0
		for n < len(q.pending) && n < fillBatchBlocks {
			if n != 0 && size+len(q.pending[n].stored.Data) > fillBatchBytes {
				break
			}
			size += len(q.pending[n].stored.Data)
			n++
		}
		batch := q.pending[:n:n]
		q.pending = q.pending[n:]
		q.bytes -= size
		for i, f := range batch {
			queued := q.queued[fillKey(f.ref)]
			queued.taken = true
			if queued.upgrade != nil {
				batch[i], queued.upgrade = *queued.upgrade, nil
			}
		}
		q.mu.Unlock()

		// Write the step; a failure drops it, as the next reads fill again.
		if err := s.writeFills(ctx, batch); err != nil {
			s.addFillStatus(0, 0, int64(len(batch)))
		}

		// Let the step's blocks queue again, now that their write ended, and
		// queue the asserted fills that waited on it.
		var dropped int64
		q.mu.Lock()
		for _, f := range batch {
			key := fillKey(f.ref)
			again := q.queued[key].again
			delete(q.queued, key)
			if again != nil && !q.pushLocked(*again) {
				dropped++
			}
		}
		q.mu.Unlock()
		if dropped != 0 {
			s.addFillStatus(0, 0, dropped)
		}
	}
}

// writeFills keeps a filled value covered by an upload marker or an eviction
// entry. With refusal retry enabled, queueing commits before any value write,
// and fillMtx holds a pass's epoch advance until every value write has
// finished, as Evict describes. A failed queue commit writes no values; a
// crash leaves only surplus queue entries, which a later pass cleans. With
// indexed accounting a marker covers a fill; every retry-on clear restores
// coverage in its marker transaction.
//
// The volume drops a cache fill whose refs it does not trust, returning
// block.ErrRefsUnknown: such a fill stores nothing, so it counts as dropped,
// not resident, and its reserved entry is removed before fillMtx releases the
// epoch, or is never queued with retry off.
//
// This store upkeep runs below Flowgraphs: it must work before a Space loads.
// Retry off retains the former value-before-queue ordering.
func (s *Store) writeFills(ctx context.Context, batch []fill) error {
	// Reserve queue coverage before exposing values, without a writer lock
	// across value I/O. Foreground block writes never acquire fillMtx.
	repairQueue := evictRefusedRetry.Load()
	if repairQueue {
		release, err := s.fillMtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer release()

		// Publish reserved coverage even if the later value write fails.
		added, err := s.queueFills(ctx, batch)
		if err != nil {
			return err
		}
		if added != 0 {
			s.addFillStatus(added, 0, 0)
		}
	}

	// Write the refilled values below the ownership layer in one batch.
	var refills []*block.PutBatchEntry
	for _, f := range batch {
		if f.refill {
			refills = append(refills, &block.PutBatchEntry{
				Ref:       f.ref,
				Data:      f.stored.Data,
				Refs:      f.stored.Refs,
				RefsKnown: f.stored.RefsSource.Asserted(),
			})
		}
	}
	if len(refills) != 0 {
		if _, err := s.values.PutBlockBatch(ctx, refills); err != nil {
			return errors.Wrap(err, "refill blocks")
		}
	}

	// Store the cache fills with their refs, all at once. A volume groups the
	// fills queued at once into one transaction, and each of its commits
	// flushes the file on a platform without a write barrier, so puts made in
	// turn would flush once per block and every durable commit of the store
	// would queue behind those flushes.
	var puts errgroup.Group
	dropped := make([]bool, len(batch))
	for i, f := range batch {
		if f.refill {
			continue
		}
		puts.Go(func() error {
			// Store the fill, and note a drop, which is not a failure.
			opts := f.stored.PutOpts(f.ref)
			opts.CacheFill = true
			_, _, err := s.local.PutBlock(ctx, f.stored.Data, opts)
			if errors.Is(err, block.ErrRefsUnknown) {
				dropped[i], err = true, nil
			}
			return err
		})
	}
	if err := puts.Wait(); err != nil {
		return errors.Wrap(err, "cache remote block")
	}

	// Split off the dropped fills, which hold nothing resident.
	var gone []fill
	if slices.Contains(dropped, true) {
		kept := make([]fill, 0, len(batch))
		for i, f := range batch {
			if dropped[i] {
				gone = append(gone, f)
			} else {
				kept = append(kept, f)
			}
		}
		batch = kept
	}

	// Remove the dropped fills' reserved coverage, or keep the legacy
	// ordering selectable, then count completed refills.
	var queued int64
	if repairQueue && len(gone) != 0 {
		removed, err := s.unqueueFills(ctx, gone)
		if err != nil {
			return err
		}
		queued = removed
	}
	if !repairQueue {
		var err error
		queued, err = s.queueFills(ctx, batch)
		if err != nil {
			return err
		}
	}
	var refilled int64
	for _, f := range batch {
		if f.refill {
			refilled++
		}
	}
	if !repairQueue || refilled != 0 || len(gone) != 0 {
		s.addFillStatus(queued, refilled, int64(len(gone)))
	}
	return nil
}

// queueFills commits eviction coverage for a fill batch and returns its added
// bytes. The caller projects them only after this transaction commits.
func (s *Store) queueFills(ctx context.Context, batch []fill) (int64, error) {
	// Count newly inserted epoch/hash entries at the current epoch.
	// The cold entry reads run without the writer lock, so
	// no writer waits on them; a commit that changed what they read meanwhile
	// runs the body again.
	var added int64
	err := kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the epoch the step queues under.
		added = 0
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}

		// Queue each value and count the bytes it adds.
		for _, f := range batch {
			n, err := queueEvict(ctx, tx, epoch, f.ref.GetHash(), int64(len(f.stored.Data)))
			if err != nil {
				return err
			}
			added += n
		}

		// Count the step's new bytes once.
		return addEvictableBytes(ctx, tx, added)
	}, kvtx.RetryInvalidSnapshot)
	if err != nil {
		return 0, errors.Wrap(err, "queue filled blocks")
	}
	return added, nil
}

// unqueueFills removes the eviction entries queueFills reserved at the current
// epoch for fills the volume dropped, and returns the bytes removed, negated.
// The caller holds fillMtx from the reservation on, so the epoch has not moved.
// A marker-covered fill reserved no entry and removes nothing. The caller
// projects the bytes only after this transaction commits.
func (s *Store) unqueueFills(ctx context.Context, gone []fill) (int64, error) {
	var removed int64
	err := kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the epoch the reservation queued under.
		removed = 0
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}

		// Delete each reserved entry and count its bytes once.
		for _, f := range gone {
			h := f.ref.GetHash()
			n, _, err := deleteEvictEntry(ctx, tx, evictEntry{key: evictKey(epoch, h), epoch: epoch, hash: h})
			if err != nil {
				return err
			}
			removed += n
		}
		return addEvictableBytes(ctx, tx, removed)
	}, kvtx.RetryInvalidSnapshot)
	if err != nil {
		return 0, errors.Wrap(err, "unqueue dropped fills")
	}
	return removed, nil
}

// addFillStatus projects a drain step or a dropped fill into the status.
func (s *Store) addFillStatus(evictable, refills, dropped int64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.EvictableBytes += evictable
		s.status.Refills += refills
		s.status.DroppedFills += dropped
		broadcast()
	})
}
