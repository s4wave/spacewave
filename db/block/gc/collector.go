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
// nets the up and down logs Land appends into the count records after the
// fact, and frees each block at zero holders once it ripens, giving the
// block's children their downs in the transaction that frees it.
//
// Between two transactions a pass checks only that ctx is live. The store
// admits a writer that waited before the pass's next transaction, as
// kvtx.WriterQueueStore requires of a background write, so nothing sleeps.
// Each transaction aims to hold the writer for sliceTarget: the pace sizes
// its budgets from the time the ones before it took, and the free loop and
// a paged release stop at the next block or page once a writer waits.
//
// A stored block is Live, with holders, or Zero, with none, on the free
// queue at its ripe sequence. It holds each child it lists in both states.
//
// Invariants. The collector keeps every one of these, and keeps a block
// whenever it is unsure:
//
//  1. Hold until free. A stored block with a child list holds each child it
//     lists until the transaction that frees it. Netting never moves a
//     child's count because its parent reached zero; the only release is a
//     free's.
//  2. Counts exact. A block's count is its named roots, the stored lists
//     that name it, its implied holder while no other holder was logged,
//     and the ups less the downs not netted yet, as Liveness.tla's
//     CountsExact states. Land logs every holder it adds or removes, a free
//     gives each child its down in the transaction that deletes the list,
//     or pages the downs in it, and a count change that would leave a block
//     below zero is refused, keeping it with one holder.
//  3. It frees only a block on the free queue with a child list and a zero
//     record at the queue entry's ripe sequence, a ripe sequence the up
//     cursor reached, and no paged release in progress, all read in the
//     transaction that frees it. It gives each child the block lists one
//     down through netting there, inline when the list fits the slice's
//     moves, and otherwise writes the list to release pages and the state's
//     Release before it commits, so a crash leaves the downs owed, never
//     lost. A child those downs take to zero takes the up log's end as its
//     ripe sequence, never its parent's.
//  4. It frees nothing while Frees is off or the Volume is not adopted. Off,
//     it still releases lapsed leases and nets the logs, so they stay
//     bounded and the counts stay current for the day Frees turns on.
//  5. Ups net promptly and whole, in page order from the up cursor,
//     whatever their age. A down page nets only once it is Grace old and
//     only in a slice that drained the up log, so every up written before it
//     nets first and a down never takes a count below a holder it was
//     written against. Grace is measured on the wall clock that stamps each down
//     page: a clock that steps forward ages the pages stamped before the
//     step early by the step, and a page stamped after the collector's
//     clock, by a clock that has since stepped back, is restamped with the
//     collector's clock when it reads it.
//  6. No late namer. A Land that holds a stored block, a root moving onto
//     it, a born block or a refilled hole naming it or a put of it again,
//     reads the block's count and dequeues it when at zero, in the
//     transaction that writes its up. The free cannot take it until that up
//     nets, which lifts it off zero or, with a down that cancels it, queues
//     it again at the up log's end. A block that reaches zero takes the up
//     log's end as its ripe sequence, so every up written before it nets
//     before the free.
//  7. No freed read. Grace is a contract on read and write sessions. A
//     reader that saw a head pins it within Grace, and a root's down nets
//     only once Grace old, so the reader finds its blocks. A block a batch
//     gives no holder takes back its implied holder in a down, and a put of
//     a block already stored holds it with an up and a down, so a session
//     that puts one tree across several batches keeps every block only
//     while its last Land, the one that holds the tree under a root or a
//     parent, follows its first batch within the grace period. A writer
//     whose session can outlast the grace period writes under a stage,
//     whose leased roots hold each block from its put. A writer that builds
//     on a root it read holds that root for the life of its write under a
//     leased pin, or the World engine's pending root does for it. Land does
//     not check that a child a put names is stored, so a writer past these
//     rules names a hole.
//  8. A child without a child list, a hole or one not yet stored, floors at
//     its implied holder when a down reaches it, and counts in Leaked; its
//     own children, unknown here, are untouched. The audit, not the
//     collector, finds what a hole leaked.
//  9. The generation rule. While the Volume's old generation is Open, as a
//     store format before 7 leaves it, it deletes nothing: an older list
//     may lack a child, cut between two slices of the old reference graph's
//     edge batch, so a count drawn from it may miss a holder. The free parks
//     each block it would free instead, giving its children no down, and
//     the settle's steps run, as settle.go describes: the run build, the
//     front's free pops, and the flip, once every unverified block is
//     unreachable. Once the generation is Sweeping, the collector moves the
//     parked blocks back onto the free queue, where invariant 3 checks each
//     again, frees what the counts prove, and leaves each unmarked block to
//     the sweep.
//  10. A parked block holds nothing. A park is a free that keeps the
//     bytes, the list and the proof: it gives each child its down in the
//     transaction that parks the block, through the free's netting, move
//     budget and paged release, and writes the block's kindReleased key,
//     so the free cascades down an unreachable subtree while Open as it
//     does once Settled. The released key is the one record that the list
//     holds nothing, and every path that would give the list a down or an
//     up reads it: the sweep and a rebirth give no down for a released list
//     and delete the key, a put gives the ups its list gives a fresh block
//     (a copied put of the stored list too), an unpark records a recount,
//     and an adoption deletes every released key before it counts every
//     stored list. The collector never requeues a released key. A count
//     then counts only the holders that are not parked, so a block whose
//     holders all parked reaches zero, ripens at the up log's end and parks
//     in the same pass. ParkKeepsHolds selects the park that kept its holds
//     until the sweep; both kinds of park may stand in one Volume, and each
//     path reads which one a block has.
type Collector struct {
	// store runs the collector's transactions.
	store Store
	// opts are the collector's options.
	opts CollectorOptions
	// mtx runs one pass at a time, and guards pace and the settle's
	// positions.
	mtx sync.Mutex
	// pace sizes each slice from the time the slices before it spent.
	pace pace
	// runFrom is the list key after which the run build resumes. Lists
	// before it that were unverified when the build passed them have run
	// entries, and no list turns unverified, so a stale one costs a rescan.
	runFrom []byte
	// sweptRun reports that a sweep pass from the start of the run reached
	// its end, so the run holds no unmarked block and the next step drops
	// it. A restart sweeps once more, finding only reborn entries.
	sweptRun bool
	// seedRoot is the root key after which the seeding resumes, seedDown the
	// down sequence at which it resumes, and seededRoots and seeded
	// report that it passed the last root and the last down page, as seed
	// describes. A restart seeds again.
	seedRoot    []byte
	seedDown    uint64
	seededRoots bool
	seeded      bool
	// front is the front scan's position, as frontPos describes. A scan that
	// reaches the front's end returns it to the head. A pass that stops at its
	// budget mid-front keeps it, so each pass reads entries the scan has not
	// yet reached, and one scan of the whole front takes as many passes as its
	// size needs instead of restarting at the head each pass, which on a
	// front larger than one pass reads never reaches the end.
	//
	// Invariant: the cursor decides only where the next front slice reads,
	// never what may be freed or when the flip may come. A front slice pops
	// an entry only on that entry's own free proof, read in the slice's
	// transaction, and the flip requires the whole front empty, read in the
	// flip's transaction. Every producer writes its entry in its own
	// transaction while it reads the generation Open: a Land's hold of an
	// unverified block (lander.holdListed), the seeding and the walk's
	// children (Records.pushOwed) and the adoption's recount
	// (adoption.hold). An entry one of them adds behind the cursor
	// during a scan therefore holds the flip until a later scan, which
	// starts at the head, pops it or the walk takes it off; an entry added
	// ahead is read in this scan; an entry removed, by the walk, a free or a
	// pop, leaves a position the next seek passes over. A position made
	// stale by any of these costs at most one partial scan. A restart begins
	// at the head, and only a scan that reached the front's end reports
	// FrontDone.
	front frontPos
	// frontIdle reports that a front scan reached the front's end since the
	// collector started, and that no event since its start could let a pop
	// succeed that the scan refused, so the settle skips the scan. A scan
	// starts at the head only when it is not idle; one that has started
	// runs to the end over as many passes as it needs.
	//
	// Invariant: a front entry's free proof, as frontProven reads it,
	// changes from false to true only by one of these, each of which ends
	// the idle:
	//  - the netting writes the block's queue entry at a ripe sequence, as
	//    it does once the block reaches zero or gains an up there, and the
	//    block is on the front, which wakeFront records as frontWake;
	//  - the up cursor reaches the ripe sequence of a block at zero, which
	//    the scan records as its position's wake and the netting as
	//    frontWake;
	//  - an adoption recounts and queues blocks at zero, and stamps a new
	//    AdoptAtUnixMilli, which frontAdoptAt records at the scan's start;
	//  - the Volume turns freeable or its recount clears, while the scan
	//    returns early and so never reaches the end.
	// A hold, a push or a park turns no proof true: a Land's hold dequeues
	// the block until its up nets, a push adds a held block or one whose
	// count the recount owes, and a park keeps the proof that a queue entry
	// gave. The one proof that turns true with no event is a damaged
	// entry's, an unlisted block at zero whose bytes then go, so a scan
	// that keeps a damaged entry does not go idle.
	// The idle decides only when the scan runs, never what it pops: each
	// pop still reads its own proof, and the flip reads the whole front.
	frontIdle bool
	// frontWake is the least ripe sequence of a front block at zero that a
	// pop refused for a cursor short of it, or that the netting queued
	// since the scan started, or 0. Once the up cursor reaches it, a pop
	// may succeed, and the idle ends.
	frontWake uint64
	// frontAdoptAt is the adoption mark under which the current or last
	// front scan started; a new adoption ends the idle.
	frontAdoptAt int64
	// frontKeys and frontEnds are the front slice's scratch: the keys it
	// read, packed end to end, and the end of each, reused across slices.
	frontKeys []byte
	frontEnds []int
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
// already bounds how long the slice holds the writer, and netting must keep
// up with the writer's appends.
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
	// removed once the transaction commits, and when rec.Unadopted reports
	// that fn returned the Volume to unadopted, the store starts an adoption
	// once it commits.
	Update(ctx context.Context, fn func(rec *Records, blocks block.StoreOps) error) error
	// View runs fn in one read snapshot of the liveness records.
	View(ctx context.Context, fn func(rec *Records) error) error
}

// NewCollector returns a collector of store.
func NewCollector(store Store, opts CollectorOptions) *Collector {
	// Fill in the defaults of the options left unset.
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
// lapsed, nets the up log and the down pages older than the grace period,
// and, when Frees is on and the Volume is adopted, frees the ripe queued
// blocks and gives their children their downs. Each step is a sequence of short transactions that
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

	// Release the roots of the lapsed leases.
	_, err = eachSlice(ctx, c.update, "", nil, func(rec *Records, blocks block.StoreOps, from string) (string, bool, error) {
		return c.sweepLeases(ctx, rec, blocks, &res, from)
	})
	if err != nil {
		return res, err
	}

	// Net and free only an adopted Volume.
	var adopted bool
	err = c.store.View(ctx, func(rec *Records) error {
		state, err := rec.GetState(ctx)
		adopted = state.GetAdoption() == Adoption_ADOPTION_ADOPTED
		return err
	})
	if err != nil || !adopted {
		return res, err
	}

	// Net, then free. A free gives its children their downs, so once a free
	// took a child to zero or started a paged release, net and free again:
	// the release continues in netting, and a child ripe at an up cursor
	// that already reached the up log's end frees in the same pass.
	for {
		if res.More, err = c.netStep(ctx, &res, past); err != nil || res.More || !c.opts.Frees {
			return res, err
		}
		again := false
		if again, res.More, err = c.freeStep(ctx, &res, past); err != nil || res.More || !again {
			return res, err
		}
	}
}

// netStep nets the logs in slices, and reports whether it stopped at past
// with work left.
func (c *Collector) netStep(ctx context.Context, res *CollectResult, past func() bool) (bool, error) {
	defer c.phase(PhaseNet)()
	return eachSlice(ctx, c.update, struct{}{}, past, func(rec *Records, _ block.StoreOps, _ struct{}) (struct{}, bool, error) {
		full, err := c.net(ctx, rec, res)
		return struct{}{}, full, err
	})
}

// freeStep moves the parked blocks back onto the queue once the generation
// flipped, frees the queued blocks, then runs the settle's steps. It reports
// whether a free took a child to zero or started a paged release, so the
// pass nets and frees again, and whether it stopped at past with work left.
// The settle runs only once the free left no work for this pass, so ripe
// blocks never wait behind a front scan or a sweep. A flip's parked blocks
// requeue in the next pass, which the flip's result asks for at once.
func (c *Collector) freeStep(ctx context.Context, res *CollectResult, past func() bool) (bool, bool, error) {
	// Requeue the parked blocks once the generation flipped.
	defer c.phase(PhaseSweep)()
	more, err := eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
		return c.requeueSlice(ctx, rec, res, from)
	})
	if err != nil || more {
		return false, more, err
	}

	// Free the queued blocks, noting whether any free gave more work.
	again := false
	more, err = eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, blocks block.StoreOps, from []byte) ([]byte, bool, error) {
		next, full, gave, err := c.freeSlice(ctx, rec, blocks, res, from)
		again = again || gave
		return next, full, err
	})
	if err != nil || more || again {
		return again, more, err
	}

	// Run the settle's steps, which while Open delete nothing.
	more, err = c.settle(ctx, past, res)
	return false, more, err
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

// sweepLeases releases up to sliceLeases roots of the leases that are not
// live, from the lease from on in ID order, in one transaction, through
// Land, so the down log records their release. It marks each lapsed lease
// released before it releases a root of it, so no renewal brings it back and
// no root joins it, and ends each lease it leaves with no root. It reports
// whether it filled the slice, and the lease the next slice starts from.
//
// Cost: one read of each lease record from from on, which is one per running
// or recently stopped process and per fill window of the last grace period;
// per released root, its index record, its entry and what Land reads and
// writes for it; per lease, one write to mark it and one to end it.
func (c *Collector) sweepLeases(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from string) (string, bool, error) {
	// Find the leases that are not live.
	now := c.opts.Now()
	limit := c.share(sliceLeases)
	leases, err := rec.LapsedLeases(ctx, from, now.UnixMilli(), limit)
	if err != nil {
		return "", false, err
	}

	// List the roots of each, up to the slice's share, marking each lease
	// released before its first release. A lease whose roots fit the share
	// ends with the slice; the next slice resumes at the last lease read.
	lapsed := make(map[string][]block.NamedRoot)
	var ended []string
	var next string
	left := limit
	for _, lease := range leases {
		if left == 0 {
			break
		}
		next = lease.ID
		if lease.Expires != 0 {
			if err := rec.ReleaseLease(ctx, lease.ID); err != nil {
				return "", false, err
			}
		}
		roots, err := rec.ListLeaseRoots(ctx, lease.ID, left)
		if err != nil {
			return "", false, err
		}
		if err := releaseRoots(ctx, rec, lease.ID, roots, lapsed); err != nil {
			return "", false, err
		}
		left -= len(roots)
		if left > 0 {
			ended = append(ended, lease.ID)
		}
	}
	more := left == 0 || len(leases) == limit
	return c.endLeases(ctx, rec, blocks, res, now, lapsed, ended, next, more)
}

// releaseRoots adds to lapsed the release of each root of roots that still
// names the lease id, and deletes the index record of one that does not.
func releaseRoots(ctx context.Context, rec *Records, id string, roots []LeasedRoot, lapsed map[string][]block.NamedRoot) error {
	for _, root := range roots {
		// Read the root.
		entry, err := rec.GetRoot(ctx, root.BucketID, root.Name)
		if err != nil {
			return err
		}
		if entry.GetLease() != id {
			if err := rec.deleteLeaseRoot(ctx, id, root.BucketID, root.Name); err != nil {
				return err
			}
			continue
		}

		// Release it from the block it holds.
		ref, _, err := ParseBlockKey(entry.GetRef())
		if err != nil {
			return err
		}
		lapsed[root.BucketID] = append(lapsed[root.BucketID], block.NamedRoot{Name: root.Name, Previous: ref})
	}
	return nil
}

// endLeases releases the lapsed roots in one Land, then deletes the records
// of the leases in ended, which hold no root once it commits, and returns
// next and more as sweepLeases does.
func (c *Collector) endLeases(
	ctx context.Context,
	rec *Records,
	blocks block.StoreOps,
	res *CollectResult,
	now time.Time,
	lapsed map[string][]block.NamedRoot,
	ended []string,
	next string,
	more bool,
) (string, bool, error) {
	// Release the roots by bucket in one Land.
	moves := make([]BucketRoots, 0, len(lapsed))
	var count int
	for _, bucketID := range slices.Sorted(maps.Keys(lapsed)) {
		moves = append(moves, BucketRoots{BucketID: bucketID, Roots: lapsed[bucketID]})
		count += len(lapsed[bucketID])
	}
	if len(moves) != 0 {
		if _, err := Land(ctx, rec, blocks, now, nil, PutHold{Mode: HeldOff}, moves...); err != nil {
			return "", false, err
		}
	}
	res.Lapsed += uint64(count) //nolint:gosec // counts are never negative.

	// End the leases left with no root.
	for _, id := range ended {
		if err := rec.EndLease(ctx, id); err != nil {
			return "", false, err
		}
	}
	return next, more, nil
}

// net runs one netting slice in rec, in an adopted Volume only: it continues
// the paged release in progress, and once that finishes nets the up pages
// from the up cursor, then, once none is left, the aged down pages, within
// the slice's budget of keys, and writes what changed. It
// reports whether the slice was full, so more work may remain.
//
// Cost: one up page read and delete per logPageKeys ups, one down page read
// and delete per logPageKeys downs, one batched count record read of the
// distinct blocks they name, one list read per block that reaches zero, and
// the count, queue and state writes that changed; a release page read per
// releasePageChildren moved children.
func (c *Collector) net(ctx context.Context, rec *Records, res *CollectResult) (bool, error) {
	// Net only an adopted Volume.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return false, err
	}

	// Continue the paged release in progress, moving one page's children
	// while a writer waits, and net nothing else until it finishes.
	moves := c.share(sliceReleases)
	if c.waiting() {
		moves = min(moves, releasePageChildren)
	}
	n, err := c.newNetting(rec, state, res, moves)
	if err != nil {
		return false, err
	}
	if err := n.continueRelease(ctx); err != nil {
		return false, err
	}
	full := state.GetRelease() != nil
	if !full {
		if full, err = c.netLogs(ctx, n, c.share(slicePairs)); err != nil {
			return false, err
		}
	}
	return full, n.write(ctx)
}

// netLogs adds the ups of the up pages from the up cursor, then the downs of
// the aged down pages, in sequence order, to n, each page whole, the first whatever its size, until budget keys, and
// deletes the pages. A first down page stamped after the collector's clock,
// which a wall clock stepped back leaves, it stamps at the clock, so it ages
// from the time the collector first saw it; the pages after it wait behind
// it, which only delays frees. It reports whether it stopped at the budget
// with pages left.
//
// Downs net only in a slice that drained the up log: it returns at the
// budget before it reads a down page while an up page is left. Every up a
// Land wrote before a down therefore nets before it, so a down never takes a
// count below a holder it was written against, and a down page needs no mark
// of the up log's position.
func (c *Collector) netLogs(ctx context.Context, n *netting, budget int) (bool, error) {
	// Read the clock once for the slice.
	rec, state := n.rec, n.state
	now := c.opts.Now().UnixMilli()

	// Net the up pages from the cursor; once none is left the cursor is the
	// up log's end.
	used, full := 0, false
	err := rec.scanUps(ctx, state.GetUpCursor(), func(seq uint64, keys []byte) (bool, error) {
		// Stop at a page that might pass the budget, unless it is the first.
		if used != 0 && used+logPageKeys > budget {
			full = true
			return false, nil
		}

		// Add each key's up, then delete the page.
		err := eachKey(keys, func(key []byte) error {
			used++
			return n.add(key, 1)
		})
		if err != nil {
			return false, err
		}
		state.UpCursor = seq + 1
		n.res.Netted++
		return true, rec.tx.Delete(ctx, rec.upKey(seq))
	})
	if err != nil || full {
		return full, err
	}
	state.UpCursor = state.GetUpSeq()

	// Net the aged down pages in order, restamping one stamped after the
	// clock.
	horizon := now - c.opts.Grace.Milliseconds()
	err = rec.scanDowns(ctx, 0, func(p downPage) (bool, error) {
		// Restamp a page stamped after the clock, and stop at one not aged
		// and one that might pass the budget unless it is the first.
		if p.at > now {
			return false, rec.restampDown(ctx, p, now)
		}
		if p.at > horizon {
			return false, nil
		}
		if used != 0 && used+logPageKeys > budget {
			full = true
			return false, nil
		}

		// Add each key's down, then delete the page.
		err := eachKey(p.keys, func(key []byte) error {
			used++
			return n.add(key, -1)
		})
		if err != nil {
			return false, err
		}
		n.res.Netted++
		return true, rec.deleteDown(ctx, p)
	})
	return full, err
}

// freeSlice frees up to sliceFrees ripe blocks at zero in one transaction,
// in ripe order, stopping at the first block the up cursor has not reached
// and at the next block once a writer waits. It frees nothing while a paged
// release is in progress or the Volume is not adopted, and frees and parks
// nothing while a recount is owed: an unpark of a released park gave its
// children no ups, so a zero count may miss a holder, and a park then would
// hide a live block. The flip reads freeable alone, so it still reaches the
// unadopt that runs the recount. A free deletes the
// block's bytes, child list, queue entry, proof and count record, and gives
// each child its down, as freeing describes; the slice stops after a free
// that starts a paged release. It reports whether the slice was full, and
// whether a free took a child to zero or started a paged release.
//
// It starts at the queue position from and returns the position past the
// entries it deleted, where the next slice of the pass resumes, so a long
// run of frees reads each entry once rather than walking the run's deleted
// entries again each slice. The resume lasts only the pass's free step:
// within it, nothing adds a queue entry before the resume except a free's
// down, which queues a child at the up log's end. The free step reports
// that, and the pass runs another from the start. A writer's Land only
// deletes queue entries and writes log pages, and netting runs before the
// free step, never between its slices.
func (c *Collector) freeSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte) ([]byte, bool, bool, error) {
	// Free only in an adopted Volume with no release in progress and no
	// recount owed.
	state, err := rec.GetState(ctx)
	if err != nil || !c.freeable(state) {
		return from, false, false, err
	}
	if owed, err := rec.recountOwed(ctx); err != nil || owed {
		return from, false, false, err
	}

	// Take the next ripe blocks, only one while a writer waits, noting one
	// that waits on its ripe sequence.
	type entry struct {
		ripe uint64
		ref  *block.BlockRef
	}
	var ripe []entry
	full := false
	cursor, frees := state.GetUpCursor(), c.share(sliceFrees)
	if c.waiting() {
		frees = 1
	}
	err = rec.scanQueue(ctx, from, func(seq uint64, ref *block.BlockRef) (bool, error) {
		switch {
		case seq > cursor:
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
		return from, false, false, err
	}

	// The frees give their children downs within the moves a netting slice
	// makes, one page's while a writer waits.
	f := &freeing{moves: c.share(sliceReleases)}
	if c.waiting() {
		f.moves = min(f.moves, releasePageChildren)
	}

	// Free each until a writer waits, a list does not fit the moves left or
	// a paged release starts; the next slice resumes there.
	for i, e := range ripe {
		if i != 0 && c.waiting() {
			full = true
			break
		}
		freed, err := c.freeBlock(ctx, rec, blocks, state, e.ripe, e.ref, res, f)
		if err != nil {
			return from, false, false, err
		}
		if !freed {
			full = true
			break
		}
		from = queuePos(e.ripe, BlockKey(e.ref))
		if state.GetRelease() != nil {
			full = true
			break
		}
	}
	return from, full, f.gave, nil
}

// requeueSlice moves up to a slice of parked blocks back onto the free queue
// once the generation flipped, from the parked key position from, and
// reports whether the slice was full. It reads only kindParked keys: a
// released park gave its downs already, so it stays for the sweep, which
// deletes its key and gives none. Each keeps its ripe sequence, which the
// cursor passed before the free parked it, so the free step that follows
// takes it, checking it is still at zero at that sequence, and drops an
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

// freeable reports whether the free may run under state: the Volume is
// adopted, Grace has passed since the adoption's mark, and no paged release
// is in progress, whose unmoved children still count their freed parent.
// The adoption settles a block no holder names at zero with no down behind
// it, so the wait gives a reader that saw it named before the mark the
// Grace a down would have, as Adopt describes.
func (c *Collector) freeable(state *LivenessState) bool {
	graced := c.opts.Now().UnixMilli()-state.GetAdoptAtUnixMilli() >= c.opts.Grace.Milliseconds()
	return state.GetAdoption() == Adoption_ADOPTION_ADOPTED && graced && state.GetRelease() == nil
}

// freeBlock frees one block at zero, queued at ripe: its bytes, child list,
// queue entry, proof and count record, and gives each child its down through
// f. It deletes only the queue entry when the entry is stale: the block has
// no child list or no zero record at ripe, or it is unmarked, which the
// sweep deletes. While the generation is Open it deletes nothing, and parks
// the block instead, as invariants 9 and 10 describe: a park gives the
// children their downs as a free does, unless ParkKeepsHolds keeps them.
// When the list does not fit
// the moves f has left after an earlier free, it changes nothing and
// reports false, so the next slice frees it first.
//
// This is the one path that deletes a block the counts prove dead. It keeps
// the invariants Collector lists: hold until free, since the children lose
// the block only here, in the transaction that deletes its list; counts
// exact, since the block's zero record at the queue's ripe is read here and
// each child moves down exactly once; no late namer, since a Land that held
// the block since it ripened deleted its queue entry, so the scan that
// found the entry proves none did; and no freed read, since the up cursor
// reached the ripe sequence, so every up before it netted.
func (c *Collector) freeBlock(
	ctx context.Context,
	rec *Records,
	blocks block.StoreOps,
	state *LivenessState,
	ripe uint64,
	ref *block.BlockRef,
	res *CollectResult,
	f *freeing,
) (bool, error) {
	// Check the block is stored.
	key := BlockKey(ref)
	queued := rec.queueKey(ripe, key)
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil {
		return false, err
	}
	if !listed {
		return true, rec.tx.Delete(ctx, queued)
	}

	// Check its count. The netting moves a block's queue entry whenever it
	// changes the block's ripe sequence, so at differs from ripe only when
	// damage left the records apart, and the block is kept.
	holders, at, err := rec.GetHolders(ctx, ref)
	if err != nil {
		return false, err
	}
	if holders != 0 || at != ripe {
		return true, rec.tx.Delete(ctx, queued)
	}

	// Park it while the generation is Open, in the run, keeping its holds
	// only as ParkKeepsHolds asks, and leave an unmarked one to the sweep.
	park := state.GetGeneration() == Generation_GENERATION_OPEN
	switch {
	case park && c.opts.ParkKeepsHolds:
		res.Parked++
		return true, rec.park(ctx, queued, rec.parkedKey(ripe, key), key)
	case state.GetGeneration() == Generation_GENERATION_SWEEPING:
		unmarked, err := rec.Unmarked(ctx, key)
		if err != nil {
			return false, err
		}
		if unmarked {
			return true, rec.tx.Delete(ctx, queued)
		}
	}

	// Read the list whose children the free releases, only once the checks
	// above let the block go, so a kept block costs no list read. Leave a
	// list that does not fit the moves left for the next slice.
	children, _, err := rec.GetChildren(ctx, ref)
	if err != nil {
		return false, err
	}
	if len(children) > f.moves-f.moved && f.moved != 0 {
		return false, nil
	}

	// Park a block while Open with its holds released, keeping its bytes,
	// list and proof, and give its children their downs as a free does.
	if park {
		res.Parked++
		if err := rec.park(ctx, queued, rec.releasedKey(ripe, key), key); err != nil {
			return false, err
		}
		return true, f.give(ctx, c, rec, state, res, key, children)
	}

	// Free its bytes and records.
	if err := rec.tx.Delete(ctx, queued); err != nil {
		return false, err
	}
	if err := blocks.RmBlock(ctx, ref); err != nil {
		return false, err
	}
	for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.DeleteProof} {
		if err := del(ctx, ref); err != nil {
			return false, err
		}
	}
	res.Freed++
	if err := rec.SetHolders(ctx, ref, 1, 0); err != nil {
		return false, err
	}

	// Give its children their downs.
	return true, f.give(ctx, c, rec, state, res, key, children)
}

// freeing is one free slice's downs. Each free gives its children their
// downs through a netting of its own, so every count change still runs
// through netting: a child taken to zero takes the up log's end as its ripe
// sequence, one below zero is kept with one holder, and one without a child
// list floors at its implied holder and counts in Leaked. A list longer than
// the moves left starts a paged release, which netting continues, and
// nothing frees until it finishes.
type freeing struct {
	// moves is the slice's budget of moved children, and moved the children
	// its frees moved.
	moves, moved int
	// gave reports that a free took a child to zero or started a paged
	// release, so the pass nets and frees again.
	gave bool
}

// give gives each child of the freed block at key its down, inline when the
// children fit the moves left, and otherwise through release pages, which a
// free starts only as the slice's first mover.
func (f *freeing) give(ctx context.Context, c *Collector, rec *Records, state *LivenessState, res *CollectResult, key []byte, children []*block.BlockRef) error {
	// Move the children in a netting of the free's own, and resolve and
	// write what they changed.
	n, err := c.newNetting(rec, state, res, f.moves-f.moved)
	if err != nil {
		return err
	}

	// Release the children, noting the queue length to tell whether a child
	// reached zero.
	queued := res.Queued
	if err := n.release(ctx, key, children); err != nil {
		return err
	}
	if err := n.write(ctx); err != nil {
		return err
	}

	// Count the moves and report whether the pass must net and free again.
	f.moved += n.moved
	f.gave = f.gave || res.Queued != queued || state.GetRelease() != nil
	return nil
}
