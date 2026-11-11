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
// Delete(b), a block without a child list is the model's hole, and the
// landed marks are the model's young set under Gen.
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
//  11. The generation rule. While the Volume's old generation is Open, as a
//     store format before 7 leaves it, it deletes nothing: an older list may
//     lack a child, cut between two slices of the old reference graph's edge
//     batch, so a count drawn from it may miss a holder. The free parks each
//     Dead block instead, and the settle's steps run, as settle.go
//     describes: the run build, the front's free pops, and the flip, once
//     every unverified block is unreachable. Once the generation is
//     Sweeping, the collector moves the parked blocks back onto the free
//     queue, where invariant 2 checks each again, frees what the counts
//     prove, and leaves each unmarked block to the sweep.
type Collector struct {
	// store runs the collector's transactions.
	store Store
	// opts are the collector's options.
	opts CollectorOptions
	// mtx runs one pass at a time, and guards pace, cascadeFrom, the lease
	// sweep's mark and the settle's positions.
	mtx sync.Mutex
	// pace sizes each slice from the time the slices before it spent.
	pace pace
	// cascadeFrom is the block key after the last cascade record a netting
	// slice loaded, where the next one starts. It orders the
	// loads only, so a stale one costs nothing but order.
	cascadeFrom []byte
	// leaseFrom is the expiry, in Unix milliseconds, below which the last
	// lease sweep released every lease, where the next sweep starts. A
	// renewal moves each held lease's index record every minute, and a
	// store such as s4db keeps the deleted records until a merge reaches
	// its bottom level, so a sweep from the start would step over all of
	// them each pass.
	leaseFrom int64
	// leaseFullAt is when the last sweep from the start of the lease index
	// began; zero before the first.
	leaseFullAt time.Time
	// runFrom is the list key after which the run build resumes. Lists
	// before it that were unverified when the build passed them have run
	// entries, and no list turns unverified, so a stale one costs a rescan.
	runFrom []byte
	// sweptRun reports that a sweep pass from the start of the run reached
	// its end, so the run holds no unmarked block and the next step drops
	// it. A restart sweeps once more, finding only reborn entries.
	sweptRun bool
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
	return c.opts.ForegroundWaiting != nil && c.opts.ForegroundWaiting()
}

// Store is the Volume a Collector runs in.
type Store interface {
	// Update runs fn in one write transaction of the Volume, with its liveness
	// records and the block bytes of the same transaction, and commits it
	// when fn changed anything. A block removed through blocks is reported
	// removed once the transaction commits, and when rec.Unadopted reports
	// that fn returned the Volume to unadopted, the store starts an adoption
	// once it commits.
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
	past := func() bool { return c.opts.Now().After(deadline) }
	defer func() { res.Duration = c.opts.Now().Sub(start) }()

	// Release the lapsed leases, from the start of the index on the first
	// pass, every leaseRescan, and when the clock stepped back past the mark.
	from, full := c.leaseFrom, false
	if c.leaseFullAt.IsZero() || start.Sub(c.leaseFullAt) >= leaseRescan || start.UnixMilli() < from {
		from, full = 0, true
	}
	var reached int64
	_, err = eachSlice(ctx, c.update, from, nil, func(rec *Records, blocks block.StoreOps, from int64) (int64, bool, error) {
		next, more, err := c.sweepLeases(ctx, rec, blocks, &res, from)
		reached = next
		return next, more, err
	})
	if err != nil {
		return res, err
	}
	c.leaseFrom = reached
	if full {
		c.leaseFullAt = start
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
		res.More, err = c.dropFills(ctx, past)
		return res, err
	}

	// Net the aged journal records, then the aged fill records.
	for _, body := range []func(context.Context, *netting, int) (bool, error){c.netSlice, c.netFillSlice} {
		end := c.phase(PhaseNet)
		res.More, err = eachSlice(ctx, c.update, struct{}{}, past, func(rec *Records, _ block.StoreOps, _ struct{}) (struct{}, bool, error) {
			full, err := c.net(ctx, rec, &res, body)
			return struct{}{}, full, err
		})
		end()
		if err != nil || res.More {
			return res, err
		}
	}

	// Run the settle's steps, then free the queued blocks, after moving the
	// parked ones back onto the queue once the generation flipped.
	if c.opts.Frees {
		defer c.phase(PhaseSweep)()
		res.More, err = c.settle(ctx, past, &res)
		if err != nil || res.More {
			return res, err
		}
		res.More, err = eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
			return c.requeueSlice(ctx, rec, &res, from)
		})
		if err != nil || res.More {
			return res, err
		}
		res.More, err = eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, blocks block.StoreOps, from []byte) ([]byte, bool, error) {
			return c.freeSlice(ctx, rec, blocks, &res, from)
		})
	}
	return res, err
}

// eachSlice runs slice in one transaction of update at a time, the first at
// the position from and each later one at the position the one before
// returned, until one reports no more work, ctx ends, or past, unless nil,
// reports true between two of them. It reports whether it stopped at past
// with work left. A store may run a transaction more than once, so slice
// reads its position only from its argument, and only the run the store
// commits moves it.
func eachSlice[P any](
	ctx context.Context,
	update func(context.Context, func(*Records, block.StoreOps) error) error,
	from P,
	past func() bool,
	slice func(rec *Records, blocks block.StoreOps, from P) (P, bool, error),
) (bool, error) {
	for {
		var next P
		var more bool
		err := update(ctx, func(rec *Records, blocks block.StoreOps) error {
			var err error
			next, more, err = slice(rec, blocks, from)
			return err
		})
		if err != nil || !more {
			return false, err
		}
		if past != nil && past() {
			return true, nil
		}
		if err := context.Cause(ctx); err != nil {
			return false, err
		}
		from = next
	}
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

// sweepLeases releases up to sliceLeases leased roots whose expiry passed and
// is at or after from, in one transaction, through Land, so the journal
// records their release. It seeks the lease index, so a pass reads only the
// lapsed leases. It reports whether it filled the slice, and the expiry the
// next slice starts from: the last released lease's when it filled the slice,
// else now, below which it released every lease from from on.
func (c *Collector) sweepLeases(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from int64) (int64, bool, error) {
	// Find the lapsed roots.
	now := c.opts.Now()
	limit := c.share(sliceLeases)
	found, err := rec.ScanLapsed(ctx, from, now.UnixMilli(), limit)
	if err != nil {
		return 0, false, err
	}

	// Release each from the block it holds, by bucket.
	lapsed := make(map[string][]block.NamedRoot)
	for _, root := range found {
		entry, err := rec.GetRoot(ctx, root.BucketID, root.Name)
		if err != nil {
			return 0, false, err
		}
		ref, _, err := ParseBlockKey(entry.GetRef())
		if err != nil {
			return 0, false, err
		}
		lapsed[root.BucketID] = append(lapsed[root.BucketID], block.NamedRoot{Name: root.Name, Previous: ref})
	}

	// Release them in one Land.
	moves := make([]BucketRoots, 0, len(lapsed))
	for _, bucketID := range slices.Sorted(maps.Keys(lapsed)) {
		moves = append(moves, BucketRoots{BucketID: bucketID, Roots: lapsed[bucketID]})
	}
	if _, err := Land(ctx, rec, blocks, now, nil, moves...); err != nil {
		return 0, false, err
	}
	res.Lapsed += uint64(len(found)) //nolint:gosec // counts are never negative.

	// Continue from the last released lease, or from now once none is left.
	if len(found) == limit {
		return found[len(found)-1].Expires, true, nil
	}
	return now.UnixMilli(), false, nil
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

// net runs one netting slice in rec, in an adopted Volume only: the steps
// startNetting takes, then body with the journal budget they left when they
// let the slice go on, then the resolve, and writes what changed and the
// state when it changed. It reports whether the slice was full.
func (c *Collector) net(ctx context.Context, rec *Records, res *CollectResult, body func(context.Context, *netting, int) (bool, error)) (bool, error) {
	// Net only an adopted Volume.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return false, err
	}

	// Take the steps before the records, then the records.
	n, budget, ok, err := c.startNetting(ctx, rec, state, res)
	if err != nil {
		return false, err
	}
	full := !ok
	if ok {
		if full, err = body(ctx, n, budget); err != nil {
			return false, err
		}
	}

	// Resolve and write the nodes, and the state when it changed.
	if err := n.resolve(ctx); err != nil {
		return false, err
	}
	if err := n.write(ctx); err != nil {
		return false, err
	}
	full = full || n.full
	start, err := n.state.MarshalVT()
	if err != nil || bytes.Equal(start, n.start) {
		return full, err
	}
	return full, rec.SetState(ctx, n.state)
}

// netSlice nets up to sliceRecords journal records from the cursor, all of
// them at least Grace old, and up to budget of their entries, as net's body:
// it sums each block's up and down pairs, notes the blocks the again lists
// name, and deletes the records it netted whole, for net to resolve with the
// cascades they start. A record with more entries than the slice has left is
// netted in part: the state's net offset counts the entries done, its up
// pairs, then its down pairs, then its again list, and the record stays at
// the cursor. It reports whether it filled the slice, so more work may
// remain.
func (c *Collector) netSlice(ctx context.Context, n *netting, budget int) (bool, error) {
	// Bound the slice by the grace period and the record budget.
	rec, state := n.rec, n.state
	now := c.opts.Now().UnixMilli()
	horizon := now - c.opts.Grace.Milliseconds()
	records, offset := c.share(sliceRecords), state.GetNetOffset()

	// Sum the entries of the aged records from the cursor.
	var netted []uint64
	partial := false
	future := make(map[uint64]*JournalRecord)
	err := rec.ScanJournal(ctx, state.GetCursor(), func(seq uint64, r *JournalRecord) (bool, error) {
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
		return false, err
	}

	// Restamp the records stamped after the collector's clock, by a clock
	// that has since stepped back, so each ages from the time the collector
	// first saw it rather than waiting for the clock to pass its stamp.
	for seq, r := range future {
		r.AtUnixMilli = now
		if err := rec.PutJournal(ctx, seq, r); err != nil {
			return false, err
		}
	}

	// Delete the records netted whole, move the cursor past them, and keep
	// the offset into the one netted in part.
	for _, seq := range netted {
		if err := rec.DeleteJournal(ctx, seq); err != nil {
			return false, err
		}
	}
	if len(netted) != 0 {
		state.Cursor = netted[len(netted)-1] + 1
	}
	state.NetOffset = offset
	n.res.Netted += uint64(len(netted))
	return partial || len(netted) == records || budget == 0, nil
}

// netFillSlice nets up to sliceRecords fill records at least Grace old as
// net's body, after the steps startNetting takes, which applied their up
// pairs: it adds the down pair of each record's own block, for net to
// resolve with the cascades it starts, and deletes the records with their
// index entries. It reports whether it filled the slice.
func (c *Collector) netFillSlice(ctx context.Context, n *netting, budget int) (bool, error) {
	// Restamp the records stamped after the collector's clock, so each ages
	// from the time the collector first saw it.
	rec := n.rec
	now := c.opts.Now().UnixMilli()
	var future []fillRecord
	err := rec.scanFillRecords(ctx, now+1, func(fr fillRecord) (bool, error) {
		future = append(future, fr)
		return len(future) < sliceRecords, nil
	})
	if err != nil {
		return false, err
	}
	for _, fr := range future {
		if err := rec.restampFillRecord(ctx, fr, now); err != nil {
			return false, err
		}
	}

	// Take the aged records from the oldest, within the slice's budgets, and
	// add their down pairs.
	horizon := now - c.opts.Grace.Milliseconds()
	records := min(c.share(sliceRecords), max(budget, 1))
	var taken []fillRecord
	full := false
	err = rec.scanFillRecords(ctx, 0, func(fr fillRecord) (bool, error) {
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
		return false, err
	}

	// Delete the records.
	for _, fr := range taken {
		if err := rec.deleteFillRecord(ctx, fr); err != nil {
			return false, err
		}
	}
	n.res.Netted += uint64(len(taken))
	return full, nil
}

// dropFills deletes the fill records of an unadopted Volume in slices until
// none is left or past reports true, and reports whether records remain.
// The adoption counts every child list the fills wrote, so their records only
// grow the store until then.
func (c *Collector) dropFills(ctx context.Context, past func() bool) (bool, error) {
	return eachSlice(ctx, c.update, struct{}{}, past, func(rec *Records, _ block.StoreOps, _ struct{}) (struct{}, bool, error) {
		// Leave the records of an adopting or adopted Volume.
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_UNADOPTED {
			return struct{}{}, false, err
		}
		limit := c.share(sliceRecords)
		n, err := rec.deleteFills(ctx, limit)
		return struct{}{}, n == limit, err
	})
}

// freeSlice frees up to sliceFrees ripe Dead blocks in one transaction, in
// ripe order, stopping at the first block the cursor has not passed and at
// the next block once a writer waits. It frees nothing while a cascade is
// due or in progress, while a fill's up pairs are unapplied, or until the
// cursor passes the hold mark, as Collector describes. A free deletes the
// block's bytes, child list, queue entry, proof and count record, and
// changes no other count: its children lost its holder when it died. It
// reports whether the slice was full.
//
// It starts at the queue position from and returns the position past the
// entries it deleted, where the next slice of the pass resumes, so a long
// run of frees reads each entry once rather than walking the run's deleted
// entries again each slice. The resume lasts only the pass's free step:
// within it, nothing adds a queue entry before the resume. The free deletes
// entries and adds none, a writer's Land and a fill write only journal and
// fill records, and the only path that adds an entry below the cursor, a
// Dying block whose cascade finishes and turns it Dead at its old ripe
// sequence, is netting, which runs before the free step, never between its
// slices. A cascade a slice finds due stops the step.
func (c *Collector) freeSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Free only in an adopted Volume with every cascade and fill up netted.
	state, err := rec.GetState(ctx)
	if err != nil {
		return from, false, err
	}
	if ok, err := rec.freeable(ctx, state); err != nil || !ok {
		return from, false, err
	}

	// Take the next ripe blocks, only one while a writer waits, noting one
	// that waits on its ripe sequence or the hold mark.
	var ripe []queueEntry
	full := false
	cursor, frees := state.GetCursor(), c.share(sliceFrees)
	if c.waiting() {
		frees = 1
	}
	err = rec.scanQueue(ctx, from, func(seq uint64, ref *block.BlockRef) (bool, error) {
		switch {
		case seq > cursor || cursor < state.GetHoldMark():
			res.Kept++
			return false, nil
		case len(ripe) == frees:
			full = true
			return false, nil
		}
		ripe = append(ripe, queueEntry{ripe: seq, ref: ref})
		return true, nil
	})
	if err != nil {
		return from, false, err
	}

	// Free each until a writer waits; the next slice resumes there.
	more := full
	for i, e := range ripe {
		if i != 0 && c.waiting() {
			more = true
			break
		}
		if err := c.freeBlock(ctx, rec, blocks, state, e, res); err != nil {
			return from, false, err
		}
		from = queuePos(e.ripe, BlockKey(e.ref))
	}
	return from, more, nil
}

// requeueSlice moves up to a slice of parked blocks back onto the free queue
// once the generation flipped, from the parked key position from, and
// reports whether the slice was full. Each keeps its ripe sequence, which the
// cursor passed before the free parked it, so the free step that follows
// takes it, checking it is still Dead at that sequence, and drops an
// unmarked one, which the sweep deletes. While the generation is Open, or the
// Volume is not adopted, it moves nothing.
func (c *Collector) requeueSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Requeue only once an adopted Volume's generation flipped.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() == Generation_GENERATION_OPEN || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return from, false, err
	}

	// Take the next parked keys, then move each.
	var keys [][]byte
	full, budget := false, c.share(sliceFrees)
	err = rec.scan(ctx, rec.key(kindParked), from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		if len(keys) == budget {
			full = true
			return false, nil
		}
		keys = append(keys, slices.Clone(key))
		return true, nil
	})
	if err != nil {
		return from, false, err
	}
	for _, key := range keys {
		if err := rec.tx.Delete(ctx, rec.key(kindParked, key)); err != nil {
			return from, false, err
		}
		if err := rec.tx.Set(ctx, rec.key(kindQueue, key), nil); err != nil {
			return from, false, err
		}
		res.Requeued++
		from = key
	}
	return from, full, nil
}

// freeable reports whether the free may run in rec, apart from the hold
// mark, which it checks per entry: the Volume is adopted, no cascade is due
// or in progress, and no fill up is unapplied. A cascade is due although
// the pass's netting found no work left when an adoption marked the Volume
// adopted since, its zero blocks Dying, or when the store lost a Holding
// block, whose re-hold waits for a put to refill its list. A fill's up pairs
// are unapplied when it landed after the last netting slice.
func (r *Records) freeable(ctx context.Context, state *LivenessState) (bool, error) {
	// Check the state, then the cascade and fill up records.
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return false, nil
	}
	settled, err := r.settled(ctx, state)
	if err != nil || !settled {
		return false, err
	}
	ups, err := r.anyKey(ctx, r.key(kindFillUps))
	return !ups, err
}

// queueEntry is a block on the free queue at its ripe sequence.
type queueEntry struct {
	// ripe is the block's ripe sequence.
	ripe uint64
	// ref is the block.
	ref *block.BlockRef
}

// freeBlock frees the Dead block of queue entry e: its bytes, child list,
// queue entry, proof and count record. It deletes only the queue entry when
// the entry is stale: the block has no child list or no zero record at e's
// ripe sequence, or it is unmarked, which the sweep deletes. While the
// generation is Open it deletes nothing, and parks the block instead, as the
// settle describes.
func (c *Collector) freeBlock(ctx context.Context, rec *Records, blocks block.StoreOps, state *LivenessState, e queueEntry, res *CollectResult) error {
	// Check the block is stored, Dead and at the entry's ripe sequence.
	ripe, ref := e.ripe, e.ref
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

	// Park it while the generation is Open, and leave an unmarked one to
	// the sweep.
	switch state.GetGeneration() {
	case Generation_GENERATION_OPEN:
		res.Parked++
		return rec.tx.Set(ctx, rec.parkedKey(ripe, key), nil)
	case Generation_GENERATION_SWEEPING:
		unmarked, err := rec.Unmarked(ctx, key)
		if err != nil || unmarked {
			return err
		}
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
