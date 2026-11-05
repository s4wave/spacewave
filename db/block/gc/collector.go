package block_gc

import (
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
//     head and has not pinned it yet still finds its blocks.
//  4. It frees no queued block that an up pair in the journal past its cursor
//     names: a later transaction added a holder to it, which the interval it
//     netted did not include, such as a root moved away and back within the
//     grace period. In each free transaction it reads the records appended
//     since its last look, so nothing lands unseen between the check and the
//     free.
//  5. A cascade decrements only the children a freed block's own child list
//     names, and only those the store keeps a child list for. A child without
//     one is a hole: its children are unknown here, so they stay. The audit,
//     not the collector, finds what a hole leaked.
//  6. A count change that would go below zero is refused, and the block it
//     names is kept with one holder: a holder vanished that the journal never
//     added, so the counts cannot be trusted for it.
//  7. A writer that builds on a root it read holds that root for the life of
//     its write under a leased pin, or the World engine's pending root does
//     for it. The collector cannot see a writer that names a block it never
//     held, so a write that names a block the collector already freed is the
//     writer's error, not a free of a live block.
type Collector struct {
	// store runs the collector's transactions.
	store Store
	// opts are the collector's options.
	opts CollectorOptions
	// mtx runs one pass at a time.
	mtx sync.Mutex
	// tail holds the block keys up pairs past the cursor name, as of the
	// journal records before tailSeq. Guarded by mtx.
	tail map[string]struct{}
	// tailSeq is the sequence number of the first record the tail has not
	// read. Guarded by mtx.
	tailSeq uint64
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
	// WritersWaiting reports whether a writer waits for the store's writer,
	// so the collector lets it in between two transactions.
	WritersWaiting() bool
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
func (c *Collector) Collect(ctx context.Context) (CollectResult, error) {
	// Run one pass at a time.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	start := c.opts.Now()
	deadline := start.Add(c.opts.Budget)
	var res CollectResult
	defer func() { res.Duration = c.opts.Now().Sub(start) }()

	// Release the lapsed leases.
	if err := c.sweepLeases(ctx, &res); err != nil {
		return res, err
	}

	// Read whether the Volume is adopted.
	var adopted bool
	err := c.store.View(ctx, func(rec *Records) error {
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

// yield lets a waiting writer take the store's writer before the next slice.
func (c *Collector) yield(ctx context.Context) error {
	if !c.store.WritersWaiting() {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(yieldPause):
		return nil
	}
}

// sweepLeases releases the leased roots whose expiry passed, sliceLeases per
// transaction, each through Land, so the journal records their release.
func (c *Collector) sweepLeases(ctx context.Context, res *CollectResult) error {
	for {
		// Release the next lapsed roots, read inside the transaction.
		var released int
		err := c.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
			// Find the lapsed roots by bucket.
			now := c.opts.Now()
			lapsed := make(map[string][]block.NamedRoot)
			err := rec.ScanRoots(ctx, func(bucketID, name string, entry *RootEntry) error {
				// Skip a root without a lapsed lease.
				expires := entry.GetExpiresUnixMilli()
				if expires == 0 || expires > now.UnixMilli() || released == sliceLeases {
					return nil
				}

				// Release it from the block it holds.
				ref, _, err := ParseBlockKey(entry.GetRef())
				if err != nil {
					return err
				}
				lapsed[bucketID] = append(lapsed[bucketID], block.NamedRoot{Name: name, Previous: ref})
				released++
				return nil
			})
			if err != nil {
				return err
			}

			// Release each bucket's lapsed roots.
			for bucketID, roots := range lapsed {
				if _, err := Land(ctx, rec, blocks, now, bucketID, nil, roots); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		res.Lapsed += uint64(released) //nolint:gosec // counts are never negative.
		if released < sliceLeases {
			return nil
		}
		if err := c.yield(ctx); err != nil {
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
		if err := c.yield(ctx); err != nil {
			return false, err
		}
	}
}

// netSlice nets up to sliceRecords journal records from the cursor, all of
// them at least Grace old, in one transaction: it sums each block's up and
// down pairs, writes the count records that changed, queues the stored
// blocks whose holders reach zero, takes off the queue those that gained a
// holder, and deletes the records it netted. It reports whether it filled the
// slice, so more aged records may remain.
func (c *Collector) netSlice(ctx context.Context, res *CollectResult) (bool, error) {
	var full bool
	err := c.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Sum the deltas of the aged records from the cursor.
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}
		horizon := c.opts.Now().Add(-c.opts.Grace).UnixMilli()
		deltas := make(map[string]int)
		var netted []uint64
		err = rec.ScanJournal(ctx, state.GetCursor(), func(seq uint64, r *JournalRecord) (bool, error) {
			// Stop at a record younger than the grace period or a full slice.
			if r.GetAtUnixMilli() > horizon || len(netted) == sliceRecords {
				return false, nil
			}

			// Add its pairs to the deltas.
			for _, up := range r.GetUp() {
				deltas[string(up)]++
			}
			for _, down := range r.GetDown() {
				deltas[string(down)]--
			}
			netted = append(netted, seq)
			return true, nil
		})
		if err != nil || len(netted) == 0 {
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

		// Delete the netted records and move the cursor past them.
		for _, seq := range netted {
			if err := rec.DeleteJournal(ctx, seq); err != nil {
				return err
			}
		}
		state.Cursor = netted[len(netted)-1] + 1
		res.Netted += uint64(len(netted))
		full = len(netted) == sliceRecords
		return rec.SetState(ctx, state)
	})
	return full, err
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

// free frees the queued blocks in slices, cascading through their children,
// until a pass over the queue frees nothing or the deadline passes, and
// reports whether queued blocks may remain to free.
func (c *Collector) free(ctx context.Context, res *CollectResult, deadline time.Time) (bool, error) {
	// Start the tail from the cursor: the records past it hold the holders
	// the netted counts do not include yet.
	defer c.phase(PhaseSweep)()
	c.tail = make(map[string]struct{})
	err := c.store.View(ctx, func(rec *Records) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		c.tailSeq = state.GetCursor()
		return nil
	})
	if err != nil {
		return false, err
	}

	// Walk the queue, again from its start after a walk that freed, since a
	// cascade queues children anywhere in it.
	for {
		freedBefore := res.Freed
		var after []byte
		for {
			next, more, err := c.freeSlice(ctx, res, after)
			if err != nil {
				return false, err
			}
			if !more {
				break
			}
			after = next
			if c.opts.Now().After(deadline) {
				return true, nil
			}
			if err := c.yield(ctx); err != nil {
				return false, err
			}
		}
		if res.Freed == freedBefore {
			return false, nil
		}
		if c.opts.Now().After(deadline) {
			return true, nil
		}
	}
}

// freeSlice examines up to sliceFrees queued blocks after the block key after
// in one transaction, and frees each that no journal record past the cursor
// adds a holder to. It returns the key of the last block examined and whether
// the slice was full.
func (c *Collector) freeSlice(ctx context.Context, res *CollectResult, after []byte) ([]byte, bool, error) {
	var last []byte
	var full bool
	err := c.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
		// Refuse to free in a Volume that is not adopted.
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}

		// Read the journal records appended since the last look.
		if err := c.readTail(ctx, rec); err != nil {
			return err
		}

		// Take the next queued blocks.
		var queued []*block.BlockRef
		err = rec.ScanQueue(ctx, after, func(ref *block.BlockRef) (bool, error) {
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

		// Free each the tail adds no holder to.
		for _, ref := range queued {
			last = BlockKey(ref)
			if _, held := c.tail[string(last)]; held {
				res.Kept++
				continue
			}
			if err := c.freeBlock(ctx, rec, blocks, ref, res); err != nil {
				return err
			}
		}
		return nil
	})
	return last, full, err
}

// readTail adds to the tail the blocks the up pairs of the journal records
// since the last look name.
func (c *Collector) readTail(ctx context.Context, rec *Records) error {
	return rec.ScanJournal(ctx, c.tailSeq, func(seq uint64, r *JournalRecord) (bool, error) {
		for _, up := range r.GetUp() {
			c.tail[string(up)] = struct{}{}
		}
		c.tailSeq = seq + 1
		return true, nil
	})
}

// freeBlock frees one queued block: its bytes, child list, count and proof,
// and decrements each child its child list names. It keeps the block when the
// queue entry turns out stale: a block without a child list is not stored,
// and one with a count record has holders.
func (c *Collector) freeBlock(ctx context.Context, rec *Records, blocks block.StoreOps, ref *block.BlockRef, res *CollectResult) error {
	// Check the block is stored and has no holder.
	children, listed, err := rec.GetChildren(ctx, ref)
	if err != nil {
		return err
	}
	count, err := rec.GetCount(ctx, ref)
	if err != nil {
		return err
	}
	if !listed || count != 0 {
		return rec.Dequeue(ctx, ref)
	}

	// Free its bytes and records.
	if err := blocks.RmBlock(ctx, ref); err != nil {
		return err
	}
	for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.Dequeue, rec.DeleteProof} {
		if err := del(ctx, ref); err != nil {
			return err
		}
	}
	res.Freed++

	// Take its holder from each child.
	for _, child := range children {
		if err := c.release(ctx, rec, child, res); err != nil {
			return err
		}
	}
	return nil
}

// release takes one holder from child, whose parent was freed, queueing it
// when it was the last. A child without a child list is a hole and keeps its
// count; one already queued keeps a holder, since a parent named it.
func (c *Collector) release(ctx context.Context, rec *Records, child *block.BlockRef, res *CollectResult) error {
	// Step over a hole.
	listed, err := rec.HasChildren(ctx, child)
	if err != nil {
		return err
	}
	if !listed {
		res.Leaked++
		return nil
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
		return rec.Enqueue(ctx, child)
	}
	return rec.SetCount(ctx, child, count-1)
}
