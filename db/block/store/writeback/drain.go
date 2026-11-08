package block_store_writeback

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

const (
	// drainBatchBlocks bounds the queue entries one drain batch carries, and
	// so the entries its clear of located blocks removes.
	drainBatchBlocks = clearLocatedBlocks
	// drainMaxBlock is the largest queued block a drain pushes. A larger one
	// stays queued for Upload, so a batch always fits one push.
	drainMaxBlock = 4 << 20
)

// DrainPush sends blocks read from the local store to a receiver that uploads
// them to the remote itself and reports each upload later, such as a storage
// node. It returns nil once the receiver holds the blocks durably, and returns
// promptly when ctx ends.
type DrainPush func(ctx context.Context, entries []*block.PutBatchEntry) error

// DrainOptions configure a Drain.
type DrainOptions struct {
	// MinAge leaves out the blocks written less than this long before the
	// pass starts, which the receiver may hold already from their own push.
	MinAge time.Duration
	// BatchBytes bounds the queued bytes one push carries. A batch holds at
	// least one block.
	BatchBytes int64
	// Regrow starts another pass once the queued bytes grow this far above
	// their lowest since the last pass ended, such as after the receiver was
	// away. Zero runs one pass.
	Regrow int64
	// Admit is called before each batch with the queued bytes the previous
	// batch handled, pushed or located, and returns once the next may start.
	// It holds the drain while it is switched off. A nil Admit admits each
	// batch at once.
	Admit UploadAdmit
	// Report receives each batch's progress, the end of the pass and each
	// failure, or is nil. It runs on the drain's goroutine.
	Report func(DrainReport)
}

// DrainReport is one step of a Drain.
type DrainReport struct {
	// Pushed counts the blocks a batch pushed.
	Pushed int64
	// PushedBytes is their queued size.
	PushedBytes int64
	// Located counts the blocks a batch cleared without a push, which the
	// remote's index already named.
	Located int64
	// LocatedBytes is their queued size.
	LocatedBytes int64
	// Done reports that a pass reached its end.
	Done bool
	// Err is the failure that ended a Run early, nil otherwise.
	Err error
}

// Drain pushes the upload backlog to a receiver that uploads it, in passes
// from the newest entry older than MinAge back to the queue's head, which
// Upload drains forward, so the two meet once. It suits a backlog that the
// remote uploads slower than the receiver. A later pass, once the backlog
// grows again, walks only the entries written since the last one started:
// those the last pass pushed may wait for their reports, and are not pushed
// twice.
//
// Invariants, which Upload's own rules already keep for every other path:
//
// A queue entry the drain pushed is never cleared by the drain. It clears only
// on the receiver's report through ClearLocated, once the remote's index names
// the block, so a push that fails, times out, or reaches a receiver that later
// loses its disk leaves the entry queued for Upload, and the local block
// stays, since eviction keeps queued blocks.
//
// The drain clears an entry itself only where the remote's index already names
// the block and the remote keeps its blocks: the rule of Upload's skip, with
// Upload's clear, which reads each entry again under the writer and counts only
// those still there. So an entry Upload or a report cleared first is not
// counted twice, and a block written again since keeps the entry it changed.
//
// It takes no writer besides that clear, one per batch of at most
// drainBatchBlocks entries, behind foreground writers. Each batch reads the
// queue in a short read transaction of its own, never one across the pass.
// Its block reads go through Upload's read gate, which yields to the store's
// own reads.
//
// A block the local store no longer holds, such as one the collector swept, is
// left queued for Upload, whose clear drops its entry unless it was written
// again.
type Drain struct {
	s    *Store
	push DrainPush
	opts DrainOptions

	// below is the queue key the next batch reads below, nil between passes.
	// It moves only past a batch that pushed or needed no push.
	below []byte
	// cutoff is the key the pass started below, and stop the key it ends at:
	// the cutoff of the pass before, nil for the first, which ends at the
	// queue's head.
	cutoff, stop []byte
	// ended is the queued bytes when the last pass ended, which the next
	// pass's growth is measured from.
	ended int64
	// held is a batch read and not yet pushed, kept across a failed push so a
	// node that is away costs no reads, with the key it moves below to.
	held *drainBatch
	// last is the queued bytes the last batch handled, for Admit.
	last int64
	// done is set once the only pass ended, with Regrow zero.
	done bool
}

// drainBatch is a batch read from the local store, waiting for its push.
type drainBatch struct {
	// entries are the blocks read, in queue order.
	entries []*block.PutBatchEntry
	// report counts them, and the blocks the batch cleared as located.
	report DrainReport
	// next is the queue key the batch after it reads below.
	next []byte
}

// NewDrain builds a Drain of s's queue through push.
func (s *Store) NewDrain(push DrainPush, opts DrainOptions) *Drain {
	return &Drain{s: s, push: push, opts: opts}
}

// Run drains the queue against remote, whose index says which blocks need no
// push, until ctx ends or a step fails. With Regrow zero it returns nil once
// the one pass is done, and at once on every later call. After a failure, a
// later call resumes where this one stopped, pushing a batch it read before.
// Calls must not overlap.
func (d *Drain) Run(ctx context.Context, remote block.StoreOps) error {
	for !d.done {
		// Start a pass below the newest entry old enough.
		if d.below == nil {
			d.cutoff = queueFloor(queueTime(time.Now().Add(-d.opts.MinAge)))
			d.below = d.cutoff
		}

		// Drain batches until the pass ends.
		for d.below != nil {
			if err := d.step(ctx, remote); err != nil {
				if ctx.Err() == nil {
					d.report(DrainReport{Err: err})
				}
				return err
			}
		}

		// Stop after one pass, or wait for the backlog to grow again.
		if d.opts.Regrow <= 0 {
			d.done = true
			return nil
		}
		if err := d.s.waitRegrow(ctx, d.ended, d.opts.Regrow); err != nil {
			return err
		}
	}
	return nil
}

// waitRegrow returns once the queued bytes grow grow above their lowest since
// they were from, as netQueued reads them, or with ctx's error.
func (s *Store) waitRegrow(ctx context.Context, from, grow int64) error {
	low := from
	for {
		// Follow the lowest queued bytes, and return on enough growth.
		queued, changed := s.netQueued()
		low = min(low, queued)
		if queued-low >= grow {
			return nil
		}

		// Wait for the next change of the status.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-changed:
		}
	}
}

// step admits and drains one batch.
func (d *Drain) step(ctx context.Context, remote block.StoreOps) error {
	// Wait for the pace, then read the next batch unless one is held.
	if d.opts.Admit != nil {
		if err := d.opts.Admit(ctx, d.last); err != nil {
			return err
		}
	}
	d.last = 0
	if d.held == nil {
		batch, next, err := d.s.queueBefore(ctx, d.below, d.stop, d.opts.BatchBytes)
		if err != nil {
			return err
		}
		if next == nil {
			queued, _ := d.s.netQueued()
			d.below, d.stop, d.ended = nil, d.cutoff, queued
			d.report(DrainReport{Done: true})
			return nil
		}
		held, err := d.read(ctx, remote, batch)
		if err != nil {
			return err
		}
		held.next = next
		d.held = held
	}

	// Push what the batch read, then move past it.
	held := d.held
	if len(held.entries) != 0 {
		if err := d.push(ctx, held.entries); err != nil {
			return errors.Wrap(err, "push backlog")
		}
	}
	d.last = held.report.PushedBytes + held.report.LocatedBytes
	d.below, d.held = held.next, nil
	d.report(held.report)
	return nil
}

// read clears the batch's entries the remote's index names and reads the rest
// from the local store, in queue order, as a batch waiting for its push. A
// failed lookup reads every block: a block pushed twice is only stored twice.
func (d *Drain) read(ctx context.Context, remote block.StoreOps, batch []pendingBlock) (*drainBatch, error) {
	// Clear the entries the remote holds already.
	out := &drainBatch{}
	located := d.s.locatedBlocks(ctx, remote, batch)
	var held, rest []pendingBlock
	for i := range batch {
		if located != nil && located[i] {
			held = append(held, batch[i])
		} else {
			rest = append(rest, batch[i])
		}
	}
	if len(held) != 0 {
		if err := d.s.clear(ctx, held); err != nil {
			return nil, err
		}
		out.report.Located, out.report.LocatedBytes = int64(len(held)), sizeOf(held)
	}

	// Read the rest as many at once as the disk's load admits, leaving out a
	// block the store no longer holds.
	read := make([]*block.PutBatchEntry, len(rest))
	err := d.s.reads.readEach(ctx, len(rest), func(ctx context.Context, i int) error {
		entry, err := d.s.readPending(ctx, &rest[i])
		read[i] = entry
		return err
	})
	if err != nil {
		return nil, err
	}
	for i, entry := range read {
		if entry != nil {
			out.entries = append(out.entries, entry)
			out.report.Pushed++
			out.report.PushedBytes += rest[i].size
		}
	}
	return out, nil
}

// report hands r to the Report option, if any.
func (d *Drain) report(r DrainReport) {
	if d.opts.Report != nil {
		d.opts.Report(r)
	}
}

// queueBefore reads the queue entries below the key before and at or above
// stop, newest first, up to maxBytes of queued blocks, and returns them in
// queue order with the key the next batch reads below. A nil stop reads down
// to the queue's head, and a nil next means no live entry is left in range.
// An entry too large to push is passed over and stays queued for Upload, so a
// batch of only those is empty. The entries walked, taken or passed over, stay
// within drainBatchBlocks, which bounds one read transaction.
func (s *Store) queueBefore(ctx context.Context, before, stop []byte, maxBytes int64) ([]pendingBlock, []byte, error) {
	// No live entry sorts below the floor, and the pass ends at stop.
	floor, _ := s.readFloor()
	if bytes.Compare(stop, floor) > 0 {
		floor = stop
	}

	// Read back from before in a read transaction of this batch's own.
	var batch []pendingBlock
	var next []byte
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Start the batch over, as a retried transaction must, and seek to
		// the newest entry at or below before.
		batch, next = nil, nil
		var batchBytes int64
		walked := 0
		it := tx.Iterate(ctx, []byte(queuePrefix), true, true)
		defer it.Close()
		if err := it.Seek(before); err != nil {
			return err
		}
		for ok := it.Valid(); ok; ok = it.Next() {
			// Stop below the floor, or once the batch walked its bound.
			key := it.Key()
			if bytes.Equal(key, before) {
				continue
			}
			if floor != nil && bytes.Compare(key, floor) < 0 || walked == drainBatchBlocks {
				break
			}
			walked++

			// Pass over an entry too large to push, and stop at a full batch.
			value, err := it.Value()
			if err != nil {
				return err
			}
			item := queueItem{key: key, value: value}
			size := item.size()
			if size > drainMaxBlock {
				next = bytes.Clone(key)
				continue
			}
			if len(batch) != 0 && batchBytes+size > maxBytes {
				break
			}

			// Take the entry.
			entry, err := item.pending()
			if err != nil {
				return err
			}
			batch = append(batch, entry)
			batchBytes += size
			next = bytes.Clone(key)
		}
		return it.Err()
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "read the queue to drain")
	}

	// Hand the batch over in queue order, the order the store wrote it.
	slices.Reverse(batch)
	return batch, next, nil
}

// sizeOf sums the queued sizes of blocks.
func sizeOf(blocks []pendingBlock) int64 {
	var n int64
	for _, b := range blocks {
		n += b.size
	}
	return n
}

// netQueued returns the bytes queued less the bytes cleared since the open,
// which moves with the queue's writes and clears but not with the count of
// an open, and a channel closed when the status changes.
func (s *Store) netQueued() (int64, <-chan struct{}) {
	var queued int64
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		queued, changed = s.netQueuedBytes, getWaitCh()
	})
	return queued, changed
}
