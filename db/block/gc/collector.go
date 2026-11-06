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
// as blocks cross zero holders, and frees the Dead blocks once they ripen.
// Journal.tla models it: Collect's lease sweep is Sweep, netSlice is Net(k)
// with the Cascade and Release steps its resolve runs, netFillSlice is
// NetFill, the fill ups startNetting applies are ApplyFill, freeBlock is
// Delete(b), and a block without a child list is the model's hole.
//
// Between two transactions a pass checks only that ctx is live. The store
// admits a writer that waited before the pass's next transaction, as
// kvtx.WriterQueueStore requires of a background write, so nothing sleeps.
// Each transaction aims to hold the writer for sliceTarget: the pace sizes
// its budgets from the time the ones before it took, and the free loop and
// a paged cascade stop at the next block or page once a writer waits.
//
// A stored block is in one of four states. Live has holders, and its
// children count it. Dying has none, and its children still count it: a
// cascade record (down) is due. Dead has none, and its children do not count
// it: it sits on the free queue at its ripe sequence. Holding has holders,
// and its children do not count it: a cascade record (up) is due. Netting
// runs a due cascade in the slice that makes it due unless the slice's
// budget is spent, as netting describes.
//
// Invariants. The collector keeps every one of these, and keeps a block
// whenever it is unsure:
//
//  1. A block with no holders holds nothing: the slice that brings a block
//     to zero takes its holder from each child, and the slice that gives a
//     Dead block a holder gives it back, so a born parent's up and a dead
//     parent's down on a shared child cancel before either is read.
//  2. It frees only a Dead block: on the free queue, with a child list and a
//     zero record at the queue entry's ripe sequence, all read in the
//     transaction that frees it. The free changes no count.
//  3. It frees nothing while Frees is off or the Volume is not adopted. Off,
//     it still releases lapsed leases and nets the journal, so the journal
//     stays bounded and the counts stay current for the day Frees turns on.
//  4. It nets only journal records at least Grace old, so a reader that saw a
//     head and has not pinned it yet still finds its blocks. Grace is measured
//     on the wall clock that stamps each record: a clock that steps forward
//     ages the records stamped before the step early by the step, and a record
//     stamped after the collector's clock, by a clock that has since stepped
//     back, is restamped with the collector's clock when it reads it.
//  5. Ripeness. A block that reaches zero, or stays there after gaining a
//     holder or being put again, takes the journal's end as its ripe
//     sequence, and is freed only once the cursor passes it. Netting reached
//     its last release only after that release aged Grace, and a writer that
//     read the block lands within Grace of the release (invariant 8), so
//     every holder within the contract is in a record before the ripe
//     sequence, and nets before the free.
//  6. The hold mark. A Land that holds a stored block the collector may have
//     at zero, because a cascade is due or in progress or the block's own
//     record is a zero record, sets the hold mark past its record, and
//     nothing frees until the cursor passes it. The block's children lost
//     its holder when it reached zero and get it back only through the
//     re-hold that runs once the record nets, which may come after a child's
//     own ripe sequence.
//  7. It frees nothing while a cascade is due or in progress, so a child is
//     never freed while a revived parent's re-hold is still due, and nothing
//     while a fill's up pairs are unapplied: netting applies them in the
//     first slice that finds them, whatever their age, so a fill holds its
//     children before any free runs. A fill's down pair, for its own block's
//     implied holder, nets once the record is Grace old.
//  8. Grace is a contract on write sessions. A block a batch gives no holder
//     takes back its implied holder in its record, and a put of a block
//     already stored names it in the record's again list, so a session that
//     puts one tree across several batches keeps every block only while its
//     last Land, the one that holds the tree under a root or a parent,
//     follows its first batch within the grace period. A writer whose
//     session can outlast the grace period writes under a stage, whose
//     leased roots hold each block from its put. A writer that builds on a
//     root it read holds that root for the life of its write under a leased
//     pin, or the World engine's pending root does for it. Land does not
//     check that a child a put names is stored, so a writer past these rules
//     names a hole.
//  9. A count change that would leave a block below zero is refused, and the
//     block is kept with one holder: a holder vanished that the journal never
//     added, so the counts cannot be trusted for it. A Dead block so kept is
//     Holding, and its re-hold gives its children their holder back.
//  10. A cascade reaches only a stored child, one with a child list. A child
//     without one is a hole or not yet stored: its count moves, since the
//     parent's Land counted it, but never below its implied holder, and its
//     own children, unknown here, are untouched. The audit, not the
//     collector, finds what a hole leaked. While the Volume is unadopted the
//     collector deletes the fill records, which the adoption's count of every
//     child list replaces; a fill that lands while the Volume adopts has its
//     block treated as born, so the adoption neither counts its children nor
//     settles it, and its record supplies both.
type Collector struct {
	// store runs the collector's transactions.
	store Store
	// opts are the collector's options.
	opts CollectorOptions
	// mtx runs one pass at a time, and guards pace and cascadeFrom.
	mtx sync.Mutex
	// pace sizes each slice from the time the slices before it spent.
	pace pace
	// cascadeFrom is the block key after the last cascade record a netting
	// slice loaded, where the next one starts. It orders the
	// loads only, so a stale one costs nothing but order.
	cascadeFrom []byte
}

// pace scales a slice's budgets by how long the slices before it took: one
// longer than sliceTarget halves the share of the base budgets the next
// takes, and one shorter than a quarter of it doubles the share, back to the
// whole.
type pace struct {
	// shift is the share as a power of two: a slice takes base >> shift.
	shift uint
}

// observe adjusts the share after a slice that took d.
func (p *pace) observe(d time.Duration) {
	switch {
	case d > sliceTarget && p.shift < paceMaxShift:
		p.shift++
	case d < sliceTarget/4 && p.shift > 0:
		p.shift--
	}
}

// update runs fn as one slice, and paces the next by the time fn itself
// took on the collector's clock. The wait behind queued writers and the
// commit's cost belong to the store: no budget shortens them, so counting
// them would hold the pace at its smallest share on a store whose commit
// alone takes longer than sliceTarget. The store may run fn more than once;
// the last run is the one it commits.
func (c *Collector) update(ctx context.Context, fn func(rec *Records, blocks block.StoreOps) error) error {
	// Time each run of fn, and pace by the last one once the store returns.
	var took time.Duration
	ran := false
	err := c.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
		start := c.opts.Now()
		err := fn(rec, blocks)
		took, ran = c.opts.Now().Sub(start), true
		return err
	})
	if ran {
		c.pace.observe(took)
	}
	return err
}

// share returns the part of base a slice starting now takes: the pace's
// share, at least one. A waiting writer does not shrink it, since the pace
// already bounds how long the slice holds the writer, and the index must
// keep up with the writer's appends for frees to run at all.
func (c *Collector) share(base int) int {
	return max(base>>c.pace.shift, 1)
}

// waiting reports that a foreground writer waits on the store.
func (c *Collector) waiting() bool {
	return c.opts.WritersWaiting != nil && c.opts.WritersWaiting()
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
	if err != nil {
		return res, err
	}
	if !adopted {
		res.More, err = c.dropFills(ctx, deadline)
		return res, err
	}

	// Net the aged journal and fill records.
	more, err := c.net(ctx, &res, deadline)
	if err == nil && !more {
		more, err = c.netFills(ctx, &res, deadline)
	}
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

// Frees reports whether the collector frees.
func (c *Collector) Frees() bool {
	return c.opts.Frees
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
		limit := c.share(sliceLeases)
		err := c.update(ctx, func(rec *Records, blocks block.StoreOps) error {
			// Find the lapsed roots.
			now := c.opts.Now()
			found, err := rec.ScanLapsed(ctx, now.UnixMilli(), limit)
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
		if released < limit {
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

// startNetting runs the steps every netting slice takes before its own
// records, in one transaction: it continues the paged cascade in progress,
// loads the cascade records due, and applies the fill records' up pairs not
// applied yet. It returns the slice and the journal budget the fill ups left,
// and whether the slice may go on to its records: only once the cascade in
// progress finished and every cascade record and fill up fit. A slice that
// may not resolves what it moved and reports itself full.
func (c *Collector) startNetting(ctx context.Context, rec *Records, state *LivenessState, res *CollectResult) (*netting, int, bool, error) {
	// Continue the paged cascade in progress, moving one page's children
	// while a writer waits.
	moves := c.share(sliceReleases)
	if c.waiting() {
		moves = min(moves, releasePageChildren)
	}
	n, err := c.newNetting(rec, state, res, moves)
	if err != nil {
		return nil, 0, false, err
	}
	if err := n.continueRelease(ctx); err != nil || state.GetRelease() != nil {
		n.full = true
		return n, 0, false, err
	}

	// Load the cascade records due.
	more, err := n.loadCascades(ctx, c.share(sliceRecords))
	if err != nil || more {
		n.full = true
		return n, 0, false, err
	}

	// Apply the fill ups not applied yet.
	budget := c.share(slicePairs)
	used, more, err := n.applyFillUps(ctx, budget)
	if err != nil || more {
		n.full = true
		return n, 0, false, err
	}
	return n, max(budget-used, 0), true, nil
}

// finishNetting resolves the slice, and writes what changed and the state
// when it changed.
func finishNetting(ctx context.Context, n *netting) error {
	// Resolve and write the nodes.
	if err := n.resolve(ctx); err != nil {
		return err
	}
	if err := n.write(ctx); err != nil {
		return err
	}
	state, err := n.state.MarshalVT()
	if err != nil || bytes.Equal(state, n.start) {
		return err
	}
	return n.rec.SetState(ctx, n.state)
}

// netSlice nets up to sliceRecords journal records from the cursor, all of
// them at least Grace old, and up to slicePairs of their entries, in one
// transaction, after the steps startNetting takes: it sums each block's up
// and down pairs, notes the blocks the again lists name, resolves the sums
// with the cascades they start, and deletes the records it netted whole. A
// record with more entries than the slice has left is netted in part: the
// state's net offset counts the entries done, its up pairs, then its down
// pairs, then its again list, and the record stays at the cursor. It reports
// whether it filled the slice, so more work may remain.
func (c *Collector) netSlice(ctx context.Context, res *CollectResult) (bool, error) {
	var full bool
	err := c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Net only an adopted Volume.
		full = false
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}

		// Take the steps before the journal.
		n, budget, ok, err := c.startNetting(ctx, rec, state, res)
		if err != nil {
			return err
		}
		if !ok {
			full = true
			return finishNetting(ctx, n)
		}

		// Bound the slice by the grace period and the record budget.
		now := c.opts.Now().UnixMilli()
		horizon := now - c.opts.Grace.Milliseconds()
		records, offset := c.share(sliceRecords), state.GetNetOffset()

		// Sum the entries of the aged records from the cursor.
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
			if len(future) != 0 || r.GetAtUnixMilli() > horizon || len(netted) == records || budget == 0 {
				return false, nil
			}

			// Add its entries after the offset, up to the budget.
			ups, downs, again := r.GetUp(), r.GetDown(), r.GetAgain()
			pairs := uint64(len(ups) + len(downs))
			total := pairs + uint64(len(again))
			end := min(total, offset+uint64(budget)) //nolint:gosec // the budget is small.
			for i := offset; i < end; i++ {
				var err error
				switch {
				case i < uint64(len(ups)):
					err = n.add(ups[i], 1)
				case i < pairs:
					err = n.add(downs[i-uint64(len(ups))], -1)
				default:
					err = n.visit(again[i-pairs], true)
				}
				if err != nil {
					return false, err
				}
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
		if err := clampFuture(ctx, rec, future, now); err != nil {
			return err
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
		if err := finishNetting(ctx, n); err != nil {
			return err
		}
		full = n.full || partial || len(netted) == records || budget == 0
		return nil
	})
	return full, err
}

// netFills nets the aged fill records in slices until none is left or the
// deadline passes, and reports whether aged records remain.
func (c *Collector) netFills(ctx context.Context, res *CollectResult, deadline time.Time) (bool, error) {
	defer c.phase(PhaseNet)()
	for {
		more, err := c.netFillSlice(ctx, res)
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

// netFillSlice nets up to sliceRecords fill records at least Grace old in
// one transaction, after the steps startNetting takes, which applied their
// up pairs: it resolves the down pair of each record's own block with the
// cascades it starts, and deletes the records with their index entries. It
// reports whether it filled the slice.
func (c *Collector) netFillSlice(ctx context.Context, res *CollectResult) (bool, error) {
	var full bool
	err := c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Net only an adopted Volume.
		full = false
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}

		// Take the steps before the records.
		n, budget, ok, err := c.startNetting(ctx, rec, state, res)
		if err != nil {
			return err
		}
		if !ok {
			full = true
			return finishNetting(ctx, n)
		}

		// Restamp the records stamped after the collector's clock, so each
		// ages from the time the collector first saw it.
		now := c.opts.Now().UnixMilli()
		var future []fillRecord
		err = rec.scanFillRecordsFrom(ctx, now+1, func(fr fillRecord) (bool, error) {
			future = append(future, fr)
			return len(future) < sliceRecords, nil
		})
		if err != nil {
			return err
		}
		for _, fr := range future {
			if err := rec.restampFillRecord(ctx, fr, now); err != nil {
				return err
			}
		}

		// Take the aged records from the oldest, within the slice's budgets,
		// and add their down pairs.
		horizon := now - c.opts.Grace.Milliseconds()
		records := min(c.share(sliceRecords), max(budget, 1))
		var taken []fillRecord
		err = rec.scanFillRecords(ctx, func(fr fillRecord) (bool, error) {
			// Stop at a young record or a full slice.
			if fr.rec.GetAtUnixMilli() > horizon {
				return false, nil
			}
			if len(taken) == records {
				full = true
				return false, nil
			}
			taken = append(taken, fr)
			for _, down := range fr.rec.GetDown() {
				if err := n.add(down, -1); err != nil {
					return false, err
				}
			}
			return true, nil
		})
		if err != nil {
			return err
		}

		// Delete the records, and resolve.
		for _, fr := range taken {
			if err := rec.deleteFillRecord(ctx, fr); err != nil {
				return err
			}
		}
		res.Netted += uint64(len(taken))
		if err := finishNetting(ctx, n); err != nil {
			return err
		}
		full = full || n.full
		return nil
	})
	return full, err
}

// dropFills deletes the fill records of an unadopted Volume in slices until
// none is left or the deadline passes, and reports whether records remain.
// The adoption counts every child list the fills wrote, so their records only
// grow the store until then.
func (c *Collector) dropFills(ctx context.Context, deadline time.Time) (bool, error) {
	for {
		// Delete the next slice while the Volume is unadopted.
		var n int
		limit := c.share(sliceRecords)
		err := c.update(ctx, func(rec *Records, _ block.StoreOps) error {
			// Leave the records of an adopting or adopted Volume.
			state, err := rec.GetState(ctx)
			if err != nil || state.GetAdoption() != Adoption_ADOPTION_UNADOPTED {
				return err
			}

			// Take the next records and delete them.
			var taken []fillRecord
			err = rec.scanFillRecords(ctx, func(fr fillRecord) (bool, error) {
				taken = append(taken, fr)
				return len(taken) < limit, nil
			})
			if err != nil {
				return err
			}
			n = len(taken)
			for _, fr := range taken {
				if err := rec.deleteFillRecord(ctx, fr); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil || n < limit {
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

// free frees the ripe Dead blocks in slices until none is left or the
// deadline passes, and reports whether ripe blocks may remain.
//
// Each slice resumes the queue at the last entry the slice before it
// deleted, so a long run of frees reads each entry once rather than walking
// the run's deleted entries again each slice. The resume lasts only this
// call: within it, nothing adds a queue entry before the resume. The free
// deletes entries and adds none, a writer's Land and a fill write only
// journal and fill records, and the only path that adds an entry below the
// cursor, a Dying block whose cascade finishes and turns it Dead at its old
// ripe sequence, is netting, which runs between calls, never between the
// slices of one. A cascade a slice finds due stops the call.
func (c *Collector) free(ctx context.Context, res *CollectResult, deadline time.Time) (bool, error) {
	defer c.phase(PhaseSweep)()
	var from []byte
	for {
		full, err := c.freeSlice(ctx, res, &from)
		if err != nil || !full {
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

// freeSlice frees up to sliceFrees ripe Dead blocks in one transaction, in
// ripe order, stopping at the first block the cursor has not passed and at
// the next block once a writer waits. It frees nothing while a cascade is
// due or in progress, while a fill's up pairs are unapplied, or until the
// cursor passes the hold mark, as Collector describes. A free deletes the
// block's bytes, child list, queue entry, proof and count record, and
// changes no other count: its children lost its holder when it died. It
// starts at the queue position *from and moves *from past the entries it
// deletes, and reports whether the slice was full.
func (c *Collector) freeSlice(ctx context.Context, res *CollectResult, from *[]byte) (bool, error) {
	var full bool
	start := *from
	err := c.update(ctx, func(rec *Records, blocks block.StoreOps) error {
		// Free only in an adopted Volume with every cascade and fill up
		// netted. A cascade is due here although the pass's netting found
		// no work left when an adoption marked the Volume adopted since,
		// its zero blocks Dying, or when the store lost a Holding block,
		// whose re-hold waits for a put to refill its list. A fill's up
		// pairs are unapplied here when it landed after the last netting
		// slice.
		full, *from = false, start
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return err
		}
		settled, err := rec.settled(ctx, state)
		if err != nil || !settled {
			return err
		}
		ups, err := rec.anyKey(ctx, rec.key(kindFillUps))
		if err != nil || ups {
			return err
		}

		// Take the next ripe blocks, only one while a writer waits, noting
		// one that waits on its ripe sequence or the hold mark.
		type entry struct {
			ripe uint64
			ref  *block.BlockRef
		}
		var ripe []entry
		cursor, frees := state.GetCursor(), c.share(sliceFrees)
		if c.waiting() {
			frees = 1
		}
		err = rec.scanQueue(ctx, start, func(seq uint64, ref *block.BlockRef) (bool, error) {
			switch {
			case seq > cursor || cursor < state.GetHoldMark():
				res.Kept++
				return false, nil
			case len(ripe) == frees:
				full = true
				return false, nil
			}
			ripe = append(ripe, entry{ripe: seq, ref: ref})
			return true, nil
		})
		if err != nil {
			return err
		}

		// Free each until a writer waits; the next slice resumes there.
		for i, e := range ripe {
			if i != 0 && c.waiting() {
				full = true
				return nil
			}
			if err := c.freeBlock(ctx, rec, blocks, e.ripe, e.ref, res); err != nil {
				return err
			}
			*from = queuePos(e.ripe, BlockKey(e.ref))
		}
		return nil
	})
	return full, err
}

// freeBlock frees one Dead block at ripe: its bytes, child list, queue
// entry, proof and count record. It deletes only the queue entry when the
// entry is stale: the block has no child list or no zero record at ripe.
func (c *Collector) freeBlock(ctx context.Context, rec *Records, blocks block.StoreOps, ripe uint64, ref *block.BlockRef, res *CollectResult) error {
	// Check the block is stored, Dead and at the entry's ripe sequence.
	key := BlockKey(ref)
	if err := rec.tx.Delete(ctx, rec.queueKey(ripe, key)); err != nil {
		return err
	}
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil || !listed {
		return err
	}

	// Check its count. The netting moves a block's queue entry whenever it
	// changes the block's ripe sequence, so at differs from ripe only when
	// damage left the records apart, and the block is kept.
	holders, at, err := rec.GetHolders(ctx, ref)
	if err != nil || holders != 0 || at != ripe {
		return err
	}

	// Free its bytes and records.
	if err := blocks.RmBlock(ctx, ref); err != nil {
		return err
	}
	for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.DeleteProof} {
		if err := del(ctx, ref); err != nil {
			return err
		}
	}
	res.Freed++
	return rec.SetHolders(ctx, ref, 1, 0)
}
