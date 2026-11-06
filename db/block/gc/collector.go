package block_gc

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// Collector frees the blocks of a Volume that no held named root reaches. It
// nets the journal Land appends after the fact, cascades through child lists
// and frees through the free queue. Journal.tla models it: Collect's lease
// sweep is Sweep, netSlice is Net(k), freeBlock is Delete(b), and a block
// without a child list is the model's hole.
//
// Between two transactions a pass checks only that ctx is live. The store
// admits a writer that waited before the pass's next transaction, as
// kvtx.WriterQueueStore requires of a background write, so nothing sleeps.
//
// Invariants. The collector keeps every one of these, and keeps a block
// whenever it is unsure:
//
//  1. It frees only a block whose netted holders are zero: it sits on the
//     free queue and its count record is zero, both read in the transaction
//     that frees it.
//  2. It frees nothing while Frees is off or the Volume is not adopted. Off,
//     it still releases lapsed leases and nets the journal, so the journal
//     stays bounded and the counts stay current for the day Frees turns on.
//  3. It nets only journal records at least Grace old, so a reader that saw a
//     head and has not pinned it yet still finds its blocks. Grace is measured
//     on the wall clock that stamps each record: a clock that steps forward
//     ages the records stamped before the step early by the step, and a record
//     stamped after the collector's clock, by a clock that has since stepped
//     back, is restamped with the collector's clock when it reads it.
//  4. It frees no queued block that an up pair in the journal past its cursor
//     names: a later transaction added a holder to it, which the interval it
//     netted did not include, such as a root moved away and back within the
//     grace period. In each free transaction it reads the records appended
//     since its last look, so nothing lands unseen between the check and the
//     free.
//  5. A cascade decrements each child a freed block's own child list names,
//     and queues only a stored child, one with a child list. A child without
//     one is a hole or not yet stored: its count drops, since the parent's
//     Land counted it, but it is never freed, and its own children, unknown
//     here, are untouched. The audit, not the collector, finds what a hole
//     leaked.
//  6. A count change that would go below zero is refused, and the block it
//     names is kept with one holder: a holder vanished that the journal never
//     added, so the counts cannot be trusted for it.
//  7. Grace is a contract on write sessions. A block a batch gives no holder
//     takes back its implied holder in its record, and a put of a block
//     already stored journals an up and a down pair, so a session that puts
//     one tree across several batches keeps every block only while its last
//     Land, the one that holds the tree under a root or a parent, follows
//     its first batch within the grace period. A writer whose session can
//     outlast the grace period writes under a stage, whose leased roots hold
//     each block from its put. A writer that builds on a root it read holds
//     that root for the life of its write under a leased pin, or the World
//     engine's pending root does for it. Land does not check that a child a
//     put names is stored, so a writer past these rules names a hole.
type Collector struct {
	// store runs the collector's transactions.
	store Store
	// opts are the collector's options.
	opts CollectorOptions
	// mtx runs one pass at a time.
	mtx sync.Mutex
}

// Store is the Volume a Collector runs in.
type Store interface {
	// Update runs fn in one write transaction of the Volume, with its liveness
	// records and the block bytes of the same transaction, and commits it
	// when fn changed anything. A block removed through blocks is reported
	// removed once the transaction commits.
	Update(ctx context.Context, fn func(rec *Records, blocks block.StoreOps) error) error
	// View runs fn in one read snapshot of the liveness records.
	View(ctx context.Context, fn func(rec *Records) error) error
}

// NewCollector returns a collector of store.
func NewCollector(store Store, opts CollectorOptions) *Collector {
	if opts.Grace == 0 {
		opts.Grace = DefaultGrace
	}
	if opts.Budget == 0 {
		opts.Budget = DefaultBudget
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Collector{store: store, opts: opts}
}

// Collect runs one bounded pass: it releases the leased roots whose lease
// lapsed, nets the journal records older than the grace period, and, when
// Frees is on and the Volume is adopted, frees the queued blocks and cascades
// through their children. Each step is a sequence of short transactions that
// let a waiting writer in between. Collect reports More when it stopped with
// work left.
func (c *Collector) Collect(ctx context.Context) (res CollectResult, err error) {
	// Run one pass at a time, and report its duration in the returned result.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	start := c.opts.Now()
	deadline := start.Add(c.opts.Budget)
	defer func() { res.Duration = c.opts.Now().Sub(start) }()

	// Release the lapsed leases.
	if err := c.sweepLeases(ctx, &res); err != nil {
		return res, err
	}

	// Read whether the Volume is adopted.
	var adopted bool
	err = c.store.View(ctx, func(rec *Records) error {
		state, err := rec.GetState(ctx)
		adopted = state.GetAdoption() == Adoption_ADOPTION_ADOPTED
		return err
	})
	if err != nil || !adopted {
		return res, err
	}

	// Net the aged journal records.
	more, err := c.net(ctx, &res, deadline)
	if err != nil || more {
		res.More = more
		return res, err
	}

	// Free the queued blocks.
	if c.opts.Frees {
		res.More, err = c.free(ctx, &res, deadline)
	}
	return res, err
}

// phase reports the start of p and returns the func that ends it.
func (c *Collector) phase(p Phase) func() {
	if c.opts.Phase == nil {
		return func() {}
	}
	return c.opts.Phase(p)
}

// sweepLeases releases the leased roots whose expiry passed, sliceLeases per
// transaction, each through Land, so the journal records their release. It
// seeks the lease index, so a pass reads only the lapsed leases.
func (c *Collector) sweepLeases(ctx context.Context, res *CollectResult) error {
	for {
		// Release the next lapsed roots, read inside the transaction.
		var released int
		err := c.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
			// Find the lapsed roots.
			now := c.opts.Now()
			found, err := rec.ScanLapsed(ctx, now.UnixMilli(), sliceLeases)
			if err != nil {
				return err
			}
			released = len(found)

			// Release each from the block it holds, by bucket.
			lapsed := make(map[string][]block.NamedRoot)
			for _, root := range found {
				entry, err := rec.GetRoot(ctx, root.BucketID, root.Name)
				if err != nil {
					return err
				}
				ref, _, err := ParseBlockKey(entry.GetRef())
				if err != nil {
					return err
				}
				lapsed[root.BucketID] = append(lapsed[root.BucketID], block.NamedRoot{Name: root.Name, Previous: ref})
			}

			// Release them in one Land.
			moves := make([]BucketRoots, 0, len(lapsed))
			for _, bucketID := range slices.Sorted(maps.Keys(lapsed)) {
				moves = append(moves, BucketRoots{BucketID: bucketID, Roots: lapsed[bucketID]})
			}
			_, err = Land(ctx, rec, blocks, now, nil, moves...)
			return err
		})
		if err != nil {
			return err
		}
		res.Lapsed += uint64(released) //nolint:gosec // counts are never negative.
		if released < sliceLeases {
			return nil
		}
		if err := context.Cause(ctx); err != nil {
			return err
		}
	}
}

// net nets the aged journal records in slices until none is left or the
// deadline passes, and reports whether aged records remain.
func (c *Collector) net(ctx context.Context, res *CollectResult, deadline time.Time) (bool, error) {
	defer c.phase(PhaseNet)()
	for {
		more, err := c.netSlice(ctx, res)
		if err != nil || !more {
			return false, err
		}
		if c.opts.Now().After(deadline) {
			return true, nil
		}
		if err := context.Cause(ctx); err != nil {
			return false, err
		}
	}
}

// netSlice nets up to slicePairs pairs of at most sliceRecords journal
// records from the cursor, all of them at least Grace old, in one
// transaction: it sums each block's up and down pairs, writes the count
// records that changed, queues the stored blocks whose holders reach zero,
// takes off the queue those that gained a holder, and deletes the records it
// netted whole. A record with more pairs than the slice has left is netted in
// part: the state's net offset counts the pairs applied, its up pairs before
// its down pairs, and the record stays at the cursor. It reports whether it
// filled the slice, so more aged records may remain.
func (c *Collector) netSlice(ctx context.Context, res *CollectResult) (bool, error) {
	var full bool
	err := c.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Net only an adopted Volume.
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}

		// Set the horizon of an aged record and the slice's pair budget.
		now := c.opts.Now().UnixMilli()
		horizon := now - c.opts.Grace.Milliseconds()
		budget, offset := slicePairs, state.GetNetOffset()

		// Sum the deltas of the aged records from the cursor.
		deltas := make(map[string]int)
		var netted []uint64
		partial := false
		future := make(map[uint64]*JournalRecord)
		err = rec.ScanJournal(ctx, state.GetCursor(), func(seq uint64, r *JournalRecord) (bool, error) {
			// Note the records stamped in the future to restamp, netting
			// nothing past the first.
			if r.GetAtUnixMilli() > now && len(future) < sliceRecords {
				future[seq] = r
				return true, nil
			}

			// Stop past a future record, at a record younger than the grace
			// period, or at a full slice.
			if len(future) != 0 || r.GetAtUnixMilli() > horizon || len(netted) == sliceRecords || budget == 0 {
				return false, nil
			}

			// Add its pairs after the offset to the deltas, up to the budget.
			ups, downs := r.GetUp(), r.GetDown()
			total := uint64(len(ups) + len(downs))
			end := min(total, offset+uint64(budget)) //nolint:gosec // the budget is small.
			for i := offset; i < end; i++ {
				if i < uint64(len(ups)) {
					deltas[string(ups[i])]++
					continue
				}
				deltas[string(downs[i-uint64(len(ups))])]--
			}
			budget -= int(end - offset) //nolint:gosec // at most the budget.

			// Stop inside a record the budget ends in, or take it whole.
			if end < total {
				offset, partial = end, true
				return false, nil
			}
			netted = append(netted, seq)
			offset = 0
			return true, nil
		})
		if err != nil {
			return err
		}
		if err := clampFuture(ctx, rec, future, now); err != nil || (len(netted) == 0 && !partial) {
			return err
		}

		// Apply each block's delta, in key order.
		for _, key := range slices.Sorted(maps.Keys(deltas)) {
			if deltas[key] == 0 {
				continue
			}
			if err := c.applyDelta(ctx, rec, []byte(key), deltas[key], res); err != nil {
				return err
			}
		}

		// Delete the records netted whole, move the cursor past them, and
		// keep the offset into the one netted in part.
		for _, seq := range netted {
			if err := rec.DeleteJournal(ctx, seq); err != nil {
				return err
			}
		}
		if len(netted) != 0 {
			state.Cursor = netted[len(netted)-1] + 1
		}
		state.NetOffset = offset
		res.Netted += uint64(len(netted))
		full = partial || len(netted) == sliceRecords || budget == 0
		return rec.SetState(ctx, state)
	})
	return full, err
}

// clampFuture restamps the journal records, stamped after now by a clock
// that has since stepped back, with now, so each ages from the time the
// collector first saw it rather than waiting for the clock to pass its stamp.
func clampFuture(ctx context.Context, rec *Records, future map[uint64]*JournalRecord, now int64) error {
	for seq, r := range future {
		r.AtUnixMilli = now
		if err := rec.PutJournal(ctx, seq, r); err != nil {
			return err
		}
	}
	return nil
}

// applyDelta adds d to the netted holders of the block at key. A block on the
// free queue has zero holders; any other has its count record plus the one
// its record implies.
func (c *Collector) applyDelta(ctx context.Context, rec *Records, key []byte, d int, res *CollectResult) error {
	// Read whether the block is queued.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return err
	}
	queued, err := rec.Queued(ctx, ref)
	if err != nil {
		return err
	}

	// Read its netted holders.
	count, err := rec.GetCount(ctx, ref)
	if err != nil {
		return err
	}
	holders := int64(count) + 1 //nolint:gosec // counts stay far below the int64 range.
	if queued {
		holders = 0
	}
	next := holders + int64(d)
	res.Counted++

	// Refuse a change below zero: keep the block with one holder.
	if next < 0 {
		res.Underflows++
		if queued {
			if err := rec.Dequeue(ctx, ref); err != nil {
				return err
			}
		}
		return rec.SetCount(ctx, ref, 0)
	}

	// Queue a stored block that lost its last holder.
	if next == 0 {
		if err := rec.SetCount(ctx, ref, 0); err != nil || queued {
			return err
		}
		listed, err := rec.HasChildren(ctx, ref)
		if err != nil || !listed {
			return err
		}
		res.Queued++
		return rec.Enqueue(ctx, ref)
	}

	// Write the holders beyond the implied one, and take a block that gained
	// one off the queue.
	if queued {
		if err := rec.Dequeue(ctx, ref); err != nil {
			return err
		}
	}
	return rec.SetCount(ctx, ref, uint64(next-1)) //nolint:gosec // next is positive.
}

// freePass is the state of one free phase.
type freePass struct {
	// tail holds the block keys the up pairs past the cursor name, as of the
	// journal records before tailSeq.
	tail map[string]struct{}
	// tailSeq is the sequence number of the first record the tail has not
	// read.
	tailSeq uint64
	// low is the lowest block key a cascade of the current walk queued, or
	// nil when it queued none.
	low []byte
}

// queued notes that a cascade queued the block at key.
func (p *freePass) queued(key []byte) {
	if p.low == nil || bytes.Compare(key, p.low) < 0 {
		p.low = key
	}
}

// free frees the queued blocks in slices, cascading through their children,
// until a walk of the queue queues nothing or the deadline passes, and
// reports whether queued blocks may remain to free. A cascade queues children
// anywhere in the queue, so the next walk starts at the lowest key the last
// one queued, not at the queue's start.
func (c *Collector) free(ctx context.Context, res *CollectResult, deadline time.Time) (bool, error) {
	// Start the tail from the cursor: the records past it hold the holders
	// the netted counts do not include yet.
	defer c.phase(PhaseSweep)()
	p := &freePass{tail: make(map[string]struct{})}
	err := c.store.View(ctx, func(rec *Records) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		p.tailSeq = state.GetCursor()
		return nil
	})
	if err != nil {
		return false, err
	}

	// Walk the queue from from, then again from the lowest key a walk's
	// cascade queued.
	var from []byte
	for {
		p.low = nil
		for {
			last, more, err := c.freeSlice(ctx, p, res, from)
			if err != nil {
				return false, err
			}
			if !more {
				break
			}
			from = append(last, 0)
			if c.opts.Now().After(deadline) {
				return true, nil
			}
			if err := context.Cause(ctx); err != nil {
				return false, err
			}
		}
		if p.low == nil {
			return false, nil
		}
		if c.opts.Now().After(deadline) {
			return true, nil
		}
		from = p.low
	}
}

// freeSlice examines up to sliceFrees queued blocks from the block key from
// in one transaction, and frees each that no journal record past the cursor
// adds a holder to, stopping after the block whose children bring the
// releases to sliceReleases. It returns the key of the last block examined
// and whether the slice was full.
func (c *Collector) freeSlice(ctx context.Context, p *freePass, res *CollectResult, from []byte) ([]byte, bool, error) {
	var last []byte
	var full bool
	err := c.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
		// Refuse to free in a Volume that is not adopted.
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}

		// Read the journal records appended since the last look.
		if err := p.readTail(ctx, rec); err != nil {
			return err
		}

		// Take the next queued blocks.
		var queued []*block.BlockRef
		err = rec.ScanQueue(ctx, from, func(ref *block.BlockRef) (bool, error) {
			if len(queued) == sliceFrees {
				full = true
				return false, nil
			}
			queued = append(queued, ref)
			return true, nil
		})
		if err != nil {
			return err
		}

		// Free each the tail adds no holder to, until the releases fill.
		var released int
		for _, ref := range queued {
			last = BlockKey(ref)
			if _, held := p.tail[string(last)]; held {
				res.Kept++
				continue
			}
			n, err := c.freeBlock(ctx, rec, blocks, p, ref, res)
			if err != nil {
				return err
			}
			if released += n; released >= sliceReleases {
				full = true
				return nil
			}
		}
		return nil
	})
	return last, full, err
}

// readTail adds to the tail the blocks the up pairs of the journal records
// since the last look name.
func (p *freePass) readTail(ctx context.Context, rec *Records) error {
	return rec.ScanJournal(ctx, p.tailSeq, func(seq uint64, r *JournalRecord) (bool, error) {
		for _, up := range r.GetUp() {
			p.tail[string(up)] = struct{}{}
		}
		p.tailSeq = seq + 1
		return true, nil
	})
}

// freeBlock frees one queued block: its bytes, child list, count and proof,
// and decrements each child its child list names, returning how many. It
// keeps the block when the queue entry turns out stale: a block no longer
// queued gained a holder, such as a child an earlier free in the same slice
// took off the queue as an underflow, a block without a child list is not
// stored, and one with a count record has holders.
func (c *Collector) freeBlock(ctx context.Context, rec *Records, blocks block.StoreOps, p *freePass, ref *block.BlockRef, res *CollectResult) (int, error) {
	// Keep a block this transaction took off the queue since its scan.
	queued, err := rec.Queued(ctx, ref)
	if err != nil || !queued {
		return 0, err
	}

	// Check the block is stored and has no holder.
	children, listed, err := rec.GetChildren(ctx, ref)
	if err != nil {
		return 0, err
	}
	count, err := rec.GetCount(ctx, ref)
	if err != nil {
		return 0, err
	}
	if !listed || count != 0 {
		return 0, rec.Dequeue(ctx, ref)
	}

	// Free its bytes and records.
	if err := blocks.RmBlock(ctx, ref); err != nil {
		return 0, err
	}
	for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.Dequeue, rec.DeleteProof} {
		if err := del(ctx, ref); err != nil {
			return 0, err
		}
	}
	res.Freed++

	// Take its holder from each child, unless a cache fill gave birth to it
	// and gave its children none.
	filled, err := rec.takeFillMark(ctx, ref)
	if err != nil || filled {
		return 0, err
	}
	for _, child := range children {
		if err := c.release(ctx, rec, p, child, res); err != nil {
			return 0, err
		}
	}
	return len(children), nil
}

// release takes one holder from child, whose parent was freed, queueing it
// when it was the last. A child without a child list loses the holder too,
// since the parent's Land counted it, but is never queued: it is not stored,
// so nothing frees it and its own children, which are unknown, are untouched.
// One already queued keeps a holder, since a parent named it.
func (c *Collector) release(ctx context.Context, rec *Records, p *freePass, child *block.BlockRef, res *CollectResult) error {
	// Take the holder from a child without a list, which only its count holds.
	listed, err := rec.HasChildren(ctx, child)
	if err != nil {
		return err
	}
	if !listed {
		res.Leaked++
		count, err := rec.GetCount(ctx, child)
		if err != nil || count == 0 {
			return err
		}
		res.Counted++
		return rec.SetCount(ctx, child, count-1)
	}

	// Refuse a release from a block already at zero holders.
	queued, err := rec.Queued(ctx, child)
	if err != nil {
		return err
	}
	if queued {
		res.Underflows++
		return rec.Dequeue(ctx, child)
	}

	// Take the holder.
	count, err := rec.GetCount(ctx, child)
	if err != nil {
		return err
	}
	res.Counted++
	if count == 0 {
		res.Queued++
		p.queued(BlockKey(child))
		return rec.Enqueue(ctx, child)
	}
	return rec.SetCount(ctx, child, count-1)
}
