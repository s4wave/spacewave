package block_store_writeback

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
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
	// refill reports a block the reference graph tracks, whose value alone
	// was evicted; any other block is a cache fill.
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
	// queued holds the hash of each block pending or being written, so a block
	// read again before its write lands is not queued twice.
	queued map[string]struct{}
	// draining records that a drain goroutine runs.
	draining bool
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
// and its eviction queue entry. It changes no ownership, so call it only for
// a block the reference graph still tracks. It returns before the write, and
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
	// Skip a block already queued, and drop a fill past the byte bound.
	q := &s.fills
	key := fillKey(f.ref)
	q.mu.Lock()
	if _, ok := q.queued[key]; ok {
		q.mu.Unlock()
		return
	}
	limit := q.limit
	if limit == 0 {
		limit = defaultFillQueueBytes
	}
	if int64(q.bytes+len(f.stored.Data)) > limit {
		q.mu.Unlock()
		s.addFillStatus(0, 0, 1)
		return
	}

	// Queue it.
	if q.queued == nil {
		q.queued = make(map[string]struct{})
	}
	q.queued[key] = struct{}{}
	q.pending = append(q.pending, f)
	q.bytes += len(f.stored.Data)
	drain := !q.draining
	q.draining = true
	q.mu.Unlock()

	// Start the drain this fill found idle.
	if drain {
		go s.drainFills()
	}
}

// drainFills writes the queued fills in bounded steps until none are left.
// Fills are disposable copies of blocks the remote holds, so their commits
// are ordered rather than flushed, and wait behind every foreground writer.
// A failed step drops its fills.
func (s *Store) drainFills() {
	ctx := kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(context.Background()))
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
		q.mu.Unlock()

		// Write the step; a failure drops it, as the next reads fill again.
		if err := s.writeFills(ctx, batch); err != nil {
			s.addFillStatus(0, 0, int64(len(batch)))
		}

		// Let the step's blocks queue again, now that their write ended.
		q.mu.Lock()
		for _, f := range batch {
			delete(q.queued, fillKey(f.ref))
		}
		q.mu.Unlock()
	}
}

// writeFills writes the values of a step, then queues them for eviction in
// one transaction. A crash between the two leaves values the eviction queue
// does not count, which the next fill of the block counts.
func (s *Store) writeFills(ctx context.Context, batch []fill) error {
	// Write the refilled values below the ownership layer in one batch.
	var refills []*block.PutBatchEntry
	for _, f := range batch {
		if f.refill {
			refills = append(refills, &block.PutBatchEntry{
				Ref:       f.ref,
				Data:      f.stored.Data,
				Refs:      f.stored.Refs,
				RefsKnown: f.stored.RefsKnown,
			})
		}
	}
	if len(refills) != 0 {
		if _, err := s.values.PutBlockBatch(ctx, refills); err != nil {
			return errors.Wrap(err, "refill blocks")
		}
	}

	// Store the cache fills with their refs. Each put is its own call, which
	// a volume groups with the fills queued beside it.
	for _, f := range batch {
		if f.refill {
			continue
		}
		opts := f.stored.PutOpts(f.ref)
		opts.CacheFill = true
		if _, _, err := s.local.PutBlock(ctx, f.stored.Data, opts); err != nil {
			return errors.Wrap(err, "cache remote block")
		}
	}

	// Queue the step's values for eviction at the current epoch, counting
	// their bytes once. The cold entry reads run without the writer lock, so
	// no writer waits on them; a commit that changed what they read meanwhile
	// runs the body again.
	var added, refilled int64
	err := kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the epoch the step queues under.
		added, refilled = 0, 0
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
			if f.refill {
				refilled++
			}
		}

		// Count the step's new bytes once.
		return addEvictableBytes(ctx, tx, added)
	}, kvtx.RetryInvalidSnapshot)
	if err != nil {
		return errors.Wrap(err, "queue filled blocks")
	}

	// Publish the committed resident bytes and refill count to watchers.
	s.addFillStatus(added, refilled, 0)
	return nil
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
