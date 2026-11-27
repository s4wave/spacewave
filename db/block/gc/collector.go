package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
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
//     child's count because its parent reached zero. The only release is a
//     free's.
//  2. Counts exact. A block's count is its named roots, the stored lists
//     that reference it, its implied holder while no other holder was logged,
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
//     it, a born block or a refilled hole referencing it or a put of it again,
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
//     not check that a child a put references is stored, so a writer past these
//     rules references a hole.
//  8. A child without a child list, a hole or one not yet stored, floors at
//     its implied holder when a down reaches it, and counts in Leaked. Its
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
	// sweepFrom is the run key after which the sweep resumes, and sweptRun
	// reports that the sweep reached the run's end, so the run holds no
	// unmarked block and the drop, resuming after dropFrom, deletes it.
	//
	// Invariant: while Sweeping no run entry is added and none turns
	// unmarked. The run's writers that add an entry or write one unmarked,
	// the park, the unpark, the run build, the copied put and the walk's
	// young lists, write only while Open, and a reopen only from Settled.
	// While Sweeping an entry moves only from unmarked to reborn, by a put,
	// or to swept, by the sweep, and the drop deletes it. So every entry
	// behind sweepFrom stays swept or reborn, and a sweep that reaches the
	// end from it has left no unmarked block, though it began mid-run; the
	// drop likewise leaves no entry behind dropFrom. A restart begins both
	// at the head, and the settle forgets both once it reads the generation
	// leave Sweeping, so the next generation sweeps its whole run.
	sweepFrom []byte
	sweptRun  bool
	dropFrom  []byte
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
	// starts at the head, pops it or the walk takes it off. An entry added
	// ahead is read in this scan. An entry removed, by the walk, a free or
	// a pop, leaves a position the next seek passes over. A position made
	// stale by any of these costs at most one partial scan. A restart
	// begins at the head, and only a scan that reached the front's end
	// reports FrontDone.
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
	//    returns early and so never reaches the end;
	//  - the netting takes a front block to one holder, which may leave an
	//    unborn block unreferenced, or a reference scan ended with a
	//    candidate it could not take, each of which frontWoke records, or
	//    a push adds a block without a list at one holder, which unbornPush
	//    records. The reference scan, not the proof, pops such an entry,
	//    and a front scan is what offers it one, as unrefScan describes.
	// A hold, any other push or a park turns no proof true: a Land's hold
	// dequeues the block until its up nets, such a push adds a held block or
	// one whose count the recount owes, and a park keeps the proof that a
	// queue entry gave. The one proof that turns true with no event is a damaged
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
	// frontWoke reports an event since the current or last front scan
	// started that may leave an unborn front block unreferenced, as
	// frontIdle lists. It ends the idle.
	frontWoke bool
	// unbornPush reports that a transaction committed since the current or
	// last front scan started pushed a block without a list at one holder,
	// as NoteUnbornPush describes. It ends the idle. Any goroutine sets it.
	unbornPush atomic.Bool
	// unref is the reference scan over the unborn front entries at one
	// holder that the front scan kept.
	unref unrefScan
	// page is the scratch of the slices viewSlices runs, reused across
	// slices.
	page keyPage
	// queueFrom is the free queue position from which the free's scan
	// resumes, nil for the head, and queueBehind the entries that the
	// collector's committed transactions wrote before it since, newest last.
	// The free takes the entries behind by key, newest first, and scans
	// from queueFrom only past entries it has not read, so each deleted
	// entry, which the store steps over until a merge drops it, costs one
	// step of one scan rather than one of every scan.
	//
	// Invariant: every queue entry the free has not taken is at or after
	// queueFrom or in queueBehind. Only these write a queue entry, each
	// through Records.enqueue: the netting, including a free's, and the
	// requeue, all in the collector's own transactions,
	// which update hands to noteQueued once they commit; and the adoption,
	// which runs outside the collector but only while the Volume is not
	// adopted, and stamps a new AdoptAtUnixMilli, which queueAdoptAt
	// compares. A Land only deletes entries. A transaction that returns an
	// error, which may still have committed, forgets the position, so the
	// next scan starts at the head. A restart starts at the head. A read
	// that reaches the queue's end moves the position to queueEnd, after
	// every entry, so each entry written later joins queueBehind.
	//
	// The position decides only where the free looks, never what it frees:
	// each entry it takes must still be on the queue in the free's
	// transaction, and freeBlock reads its count and ripe sequence there.
	// An entry the position missed would wait, not go: the flip reads the
	// queue from a snapshot of its own, as flip describes.
	queueFrom    []byte
	queueBehind  []queuedBlock
	queueAdoptAt int64
	// requeueFrom is the parked key position at which the requeue resumes,
	// and requeued reports that it reached the parked keys' end since the
	// generation left Open. Only a free while Open parks, so no parked key
	// appears until the Volume is Open again, which resets both.
	requeueFrom []byte
	requeued    bool
	// lastGen is the generation the last settle read. The settle forgets
	// its positions when it reads the generation leave Sweeping, as
	// restartSettle describes.
	lastGen Generation
	// res is the running pass's result, for update's accounting.
	res *CollectResult
}

// queueBehindMax bounds Collector.queueBehind. When more entries are
// written behind the scan's position, the position moves back to the
// earliest of them, and the next scan reads forward from there.
const queueBehindMax = 1 << 16

// queueEnd is a free queue position after every entry: a ripe sequence of
// all ones, which no up sequence reaches. The free moves its position there
// once a read reaches the queue's end, so every entry written later sorts
// behind it and reaches queueBehind, and the next read finds nothing to
// step over.
var queueEnd = binary.BigEndian.AppendUint64(nil, math.MaxUint64)

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
// alone takes longer than sliceTarget. The store may run fn more than once. The
// last run is the one it commits.
//
// It notes the queue entries the committed run wrote, or forgets the free
// queue's position when the store returns an error, as Collector.queueFrom
// describes, and adds the slice and the store's share of its time to the
// pass's result.
func (c *Collector) update(ctx context.Context, fn func(rec *Records, blocks block.StoreOps) error) error {
	return c.updateThen(ctx, fn, nil)
}

// updateThen runs fn as update does, and calls then, unless nil, once the
// store committed, before it notes the run's queue entries: a free moves
// its position there, so an entry the slice queued behind the new position
// lands in queueBehind.
func (c *Collector) updateThen(ctx context.Context, fn func(rec *Records, blocks block.StoreOps) error, then func()) error {
	// Time each run of fn, and pace by the last one once the store returns.
	var took time.Duration
	var last *Records
	begin := c.opts.Now()
	err := c.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
		start := c.opts.Now()
		err := fn(rec, blocks)
		took, last = c.opts.Now().Sub(start), rec
		return err
	})
	if last != nil {
		c.pace.observe(took)
	}
	if res := c.res; res != nil {
		res.Slices++
		res.StoreTime += c.opts.Now().Sub(begin) - took
	}

	// Learn where the committed run queued blocks.
	switch {
	case err != nil:
		c.forgetQueue()
	case last != nil:
		if then != nil {
			then()
		}
		c.noteQueued(last.queued)
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
	// ViewBlocks runs fn with records and raw block reads from the same read-only snapshot.
	// The callback must not open another transaction on this store.
	ViewBlocks(ctx context.Context, fn func(rec *Records, blocks block.StoreOps) error) error
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
	c.res = &res
	defer func() {
		res.Duration, res.PaceShift, c.res = c.opts.Now().Sub(start), c.pace.shift, nil
	}()

	// Release the roots of the lapsed leases.
	var leaseFrom string
	var lapsed lapsedPage
	_, err = viewSlices(ctx, c, &leaseFrom, nil, &lapsed, func(rec *Records, page *lapsedPage, from string) error {
		return page.read(ctx, rec, c.opts.Now(), from, c.share(sliceLeases))
	}, func(rec *Records, blocks block.StoreOps, from string, page *lapsedPage) (string, bool, error) {
		return c.sweepLeases(ctx, rec, blocks, &res, page)
	})
	if err != nil {
		return res, err
	}

	// Establish that Land and the flip can skip the hole search even when
	// the Volume is unadopted or this pass does not free.
	if err := c.clearHoles(ctx); err != nil {
		return res, err
	}

	// Net and free only an adopted Volume. Find an unset down cursor's
	// floor in the snapshot so the writer need not seek over old pages.
	var adopted bool
	var downFloor uint64
	err = c.store.View(ctx, func(rec *Records) error {
		// Skip a Volume that cannot net or already has a down cursor.
		state, err := rec.GetState(ctx)
		adopted = state.GetAdoption() == Adoption_ADOPTION_ADOPTED
		if err != nil || !adopted || state.GetDownCursor() != 0 || state.GetDownSeq() == 0 {
			return err
		}

		// Read the first live page's sequence, or the log's end.
		downFloor, err = rec.downCursor(ctx, state)
		return err
	})
	if err != nil || !adopted {
		return res, err
	}

	// Record a floor beyond zero before any netting takes the writer.
	if downFloor != 0 {
		if err := c.setDownFloor(ctx, downFloor); err != nil {
			return res, err
		}
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

// setDownFloor initializes the down cursor to the floor read in a snapshot.
// No live page has a smaller sequence: appendLogs only appends at rising
// sequences, netLogs only deletes pages, and a readoption clears the log before
// it restarts both cursors at their ends. A cursor written since the snapshot
// already carries its own floor.
func (c *Collector) setDownFloor(ctx context.Context, floor uint64) error {
	return c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Keep a cursor another pass or an adoption has already advanced.
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTED || state.GetDownCursor() != 0 {
			return err
		}

		// Publish the floor before the first writer seek.
		state.DownCursor = floor
		return rec.SetState(ctx, state)
	})
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
	// Requeue the parked blocks once the generation flipped, resuming where
	// the last requeue stopped, until one reaches the end.
	defer c.phase(PhaseSweep)()
	if !c.requeued {
		var reached bool
		more, err := viewSlices(ctx, c, &c.requeueFrom, past, &c.page, func(rec *Records, page *keyPage, from []byte) error {
			return rec.readKeys(ctx, page, kindParked, from, c.share(sliceFrees), nil)
		}, func(rec *Records, _ block.StoreOps, from []byte, page *keyPage) ([]byte, bool, error) {
			next, more, end, err := c.requeueSlice(ctx, rec, res, from, page)
			reached = end
			return next, more, err
		})
		if err != nil || more {
			return false, more, err
		}
		c.requeued = reached
	}

	// Free the queued blocks, noting whether any free gave more work.
	again, more, err := c.free(ctx, res, past)
	if err != nil || more || again {
		return again, more, err
	}

	// Run the settle's steps, which while Open delete nothing.
	more, err = c.settle(ctx, past, res)
	return false, more, err
}

// free frees the ripe queued blocks in slices, each proposed by a scan in a
// read snapshot and taken in one write transaction, until no ripe block is
// left, ctx ends, or past reports true between two slices. It reports
// whether a free took a child to zero or started a paged release, and
// whether it stopped at past with work left.
//
// Cost: per slice, one read snapshot that reads the entries behind the
// position from memory and the queue's entries from queueFrom up to the
// slice's frees and the first not yet ripe, stepping over the entries
// deleted since the last scan passed there; and one write transaction of
// point reads and writes, one queue read per entry and what freeBlock
// does. The writer is held only for the write transaction, whose work the
// pace bounds, so no read under it grows with the queue's deleted entries.
// A read that proposes nothing still moves the position, to the first
// unripe entry or past the queue's end, so an idle pass steps over no
// deleted entry: it reads at most the one unripe entry it stopped at.
func (c *Collector) free(ctx context.Context, res *CollectResult, past func() bool) (bool, bool, error) {
	again := false
	for {
		// Find the ripe entries.
		start := c.opts.Now()
		scan, err := c.freeCandidates(ctx, res)
		res.ScanTime += c.opts.Now().Sub(start)
		if err != nil {
			c.forgetQueue()
			return again, false, err
		}

		// With nothing ripe, move the position to where the read stopped:
		// no transaction ran since the snapshot, so it holds every entry
		// before that point.
		if len(scan.cands) == 0 {
			c.advanceQueue(scan, 0)
			return again, false, nil
		}

		// Take them, and once the slice commits move the position past what
		// it took.
		var took, done int
		var gave bool
		err = c.updateThen(ctx, func(rec *Records, blocks block.StoreOps) error {
			var err error
			took, done, gave, err = c.freeSlice(ctx, rec, blocks, res, scan.cands)
			return err
		}, func() { c.advanceQueue(scan, took) })
		if err != nil {
			return again, false, err
		}
		again = again || gave
		if took == 0 && done == 0 {
			return again, false, nil
		}
		if past() {
			return again, true, nil
		}
		if err := context.Cause(ctx); err != nil {
			return again, false, err
		}
	}
}

// freeScan is the entries a scan proposed to one free slice.
type freeScan struct {
	// cands are the entries, the first behind of them taken from
	// queueBehind and the rest read from queueFrom on.
	cands  []queuedBlock
	behind int
	// end is where the next read starts once the slice took every
	// candidate: the first entry not yet ripe, or the last entry read when
	// the slice filled, or queueEnd when the read reached the queue's end;
	// nil when no read ran.
	end []byte
}

// freeCandidates proposes up to a slice of free queue entries ripe at the
// up cursor, one while a writer waits: the entries behind the scan's
// position newest first, which it pops from queueBehind, then the entries
// it reads from queueFrom on in a read snapshot. It proposes none when the
// free may not run, as freeSlice decides. A new adoption mark forgets the
// position.
func (c *Collector) freeCandidates(ctx context.Context, res *CollectResult) (freeScan, error) {
	var scan freeScan
	err := c.store.View(ctx, func(rec *Records) error {
		// Propose nothing when the free may not run.
		state, err := rec.GetState(ctx)
		if err != nil || !c.freeable(state) {
			return err
		}
		if owed, err := rec.recountOwed(ctx); err != nil || owed {
			return err
		}
		if at := state.GetAdoptAtUnixMilli(); at != c.queueAdoptAt {
			c.forgetQueue()
			c.queueAdoptAt = at
		}
		if state.GetGeneration() == Generation_GENERATION_OPEN {
			c.requeueFrom, c.requeued = nil, false
		}

		// Take the ripe entries behind the position, newest first.
		cursor, frees := state.GetUpCursor(), c.share(sliceFrees)
		if c.waiting() {
			frees = 1
		}
		var kept []queuedBlock
		for _, q := range slices.Backward(c.queueBehind) {

			if q.ripe <= cursor && len(scan.cands) < frees {
				scan.cands = append(scan.cands, q)
				continue
			}
			kept = append(kept, q)
		}
		slices.Reverse(kept)
		c.queueBehind, scan.behind = kept, len(scan.cands)
		if len(scan.cands) == frees {
			return nil
		}

		// Read on from the position, noting a read that reaches the end.
		stopped := false
		err = rec.scanQueue(ctx, c.queueFrom, func(seq uint64, ref *block.BlockRef) (bool, error) {
			// Note how far the read got, and stop at an unripe entry or a
			// full slice.
			q := queuedBlock{ripe: seq, key: BlockKey(ref)}
			scan.end = q.pos()
			if seq > cursor {
				res.Kept++
				stopped = true
				return false, nil
			}
			scan.cands = append(scan.cands, q)
			stopped = len(scan.cands) == frees
			return !stopped, nil
		})
		if err == nil && !stopped {
			scan.end = queueEnd
		}
		return err
	})
	return scan, err
}

// advanceQueue moves the free queue's position past what a committed free
// slice took of scan: took of its entries, a prefix. It returns the entries
// from behind the position that the slice left to queueBehind, and moves
// queueFrom forward to the first read entry the slice left, or to the
// scan's end when it took them all.
func (c *Collector) advanceQueue(scan freeScan, took int) {
	switch {
	case took < scan.behind:
		left := slices.Clone(scan.cands[took:scan.behind])
		slices.Reverse(left)
		c.queueBehind = append(c.queueBehind, left...)
	case took < len(scan.cands):
		c.queueFrom = scan.cands[took].pos()
	case scan.end != nil:
		c.queueFrom = scan.end
	}
}

// noteQueued records the queue entries a committed transaction wrote that
// sort before the free's position, so the free takes them by key, as
// Collector.queueFrom describes. Past queueBehindMax it moves the position
// back to the earliest entry behind it instead.
func (c *Collector) noteQueued(queued []queuedBlock) {
	for _, q := range queued {
		if c.queueFrom != nil && bytes.Compare(q.pos(), c.queueFrom) < 0 {
			c.queueBehind = append(c.queueBehind, q)
		}
	}
	if len(c.queueBehind) <= queueBehindMax {
		return
	}
	for _, q := range c.queueBehind {
		if pos := q.pos(); bytes.Compare(pos, c.queueFrom) < 0 {
			c.queueFrom = pos
		}
	}
	c.queueBehind = nil
}

// forgetQueue returns the free's scan to the queue's head, which holds
// every entry.
func (c *Collector) forgetQueue() {
	c.queueFrom, c.queueBehind = nil, nil
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

// NoteUnbornPush tells the collector that a committed transaction pushed
// onto the front a block without a list at one holder, as
// Records.PushedUnborn reports, so the next pass scans the front and offers
// the entry to the reference scan. Any goroutine may call it. A note for a
// push the scan already read costs one front scan. With the reference scan
// switched off no pop follows such a push, so the note is dropped.
func (c *Collector) NoteUnbornPush() {
	if !c.opts.KeepUnreferenced {
		c.unbornPush.Store(true)
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

// lapsedPage is a lease sweep slice's leases as a read snapshot found them:
// the clock it read them at, the leases not live then from the slice's
// position on, the roots the snapshot's index named for each, and whether
// more may follow.
type lapsedPage struct {
	now    time.Time
	leases []LapsedLease
	roots  [][]LeasedRoot
	more   bool
}

// read fills p in rec, a read snapshot, with up to limit roots of the
// leases not live at now, from the lease from on in ID order. It reads each
// lease record from from on, so it steps over every lease record deleted
// since the last merge, which is why it runs in a snapshot that holds no
// writer.
func (p *lapsedPage) read(ctx context.Context, rec *Records, now time.Time, from string, limit int) error {
	// Read the lapsed leases, then the roots each holds, up to limit.
	leases, err := rec.LapsedLeases(ctx, from, now.UnixMilli(), limit)
	if err != nil {
		return err
	}
	p.now, p.leases, p.roots = now, p.leases[:0], p.roots[:0]
	left := limit
	for _, lease := range leases {
		if left == 0 {
			break
		}
		roots, err := rec.ListLeaseRoots(ctx, lease.ID, left)
		if err != nil {
			return err
		}
		p.leases, p.roots = append(p.leases, lease), append(p.roots, roots)
		left -= len(roots)
	}
	p.more = left == 0 || len(leases) == limit
	return nil
}

// sweepLeases releases the roots of the leases page names, which a read
// snapshot found not live, through Land, so the down log records their
// release, and returns the lease the next slice starts from and whether more
// may follow. It decides from point reads: it skips a lease its transaction
// reads live again or gone, marks each lapsed lease released before it
// releases a root of it, so no renewal brings it back and no root joins it,
// and releases only a root whose entry still names the lease.
//
// It ends a lease only when the snapshot read it released and named no root
// of it. The release committed before that snapshot, and Land refuses a
// root onto a released lease, so no root joined it since. The next slice
// starts at the first lease this one released, so each ends in a later
// slice of the same pass.
//
// Cost: per lease, one point read and at most two writes; per released
// root, its entry and what Land reads and writes for it. The snapshot read
// steps over the deleted lease records, holding no writer.
func (c *Collector) sweepLeases(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, page *lapsedPage) (string, bool, error) {
	// Confirm each lease still lapsed, mark it released, and gather the
	// release of its roots.
	lapsed := make(map[string][]block.NamedRoot)
	var ended []string
	var next, released string
	more, now := page.more, page.now
	for i, lease := range page.leases {
		next = lease.ID
		expires, found, err := rec.GetLease(ctx, lease.ID)
		if err != nil {
			return "", false, err
		}
		if !found || expires > now.UnixMilli() {
			continue
		}
		roots := page.roots[i]
		if lease.Expires == 0 && len(roots) == 0 {
			ended = append(ended, lease.ID)
			continue
		}
		if expires != 0 {
			if err := rec.ReleaseLease(ctx, lease.ID); err != nil {
				return "", false, err
			}
		}
		if err := releaseRoots(ctx, rec, lease.ID, roots, lapsed); err != nil {
			return "", false, err
		}
		if released == "" {
			released = lease.ID
		}
	}
	if released != "" {
		next, more = released, true
	}
	return c.endLeases(ctx, rec, blocks, res, now, lapsed, ended, next, more)
}

// releaseRoots adds to lapsed the release of each root of roots that still
// references the lease id, and deletes the index record of one that does not.
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
// distinct blocks they reference, one list read per block that reaches zero,
// and the count, queue and state writes that changed; a release page read per
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
// the aged down pages, in sequence order, to n, each page whole, the first
// whatever its size, until budget keys, and deletes the pages. A first down
// page stamped after the collector's clock, which a wall clock stepped back
// leaves, it stamps at the clock, so it ages from the time the collector first
// saw it. The pages after it wait behind it, which only delays frees. It
// reports whether it stopped at the budget with pages left.
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

	// Net the up pages from the cursor. Once none is left the cursor is the
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

	// Net the aged down pages in order from the down cursor, restamping one
	// stamped after the clock. The cursor moves to the first page left, so
	// the next slice's seek lands on it, past the pages this one deletes.
	horizon := now - c.opts.Grace.Milliseconds()
	from := state.GetDownCursor()
	state.DownCursor = state.GetDownSeq()
	err = rec.scanDowns(ctx, from, func(p downPage) (bool, error) {
		// Restamp a page stamped after the clock, and stop at one not aged
		// and one that might pass the budget unless it is the first.
		state.DownCursor = p.seq
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
		state.DownCursor = p.seq + 1
		return true, rec.deleteDown(ctx, p)
	})
	return full, err
}

// freeSlice frees the blocks of cands in order, in one transaction, while
// the Volume is adopted, Grace has passed since the adoption, no paged
// release is in progress and no recount is owed: an unpark of a released
// park gave its children no ups, so a zero count may miss a holder, and a
// park then would hide a live block. The flip reads freeable alone, so it
// still reaches the unadopt that runs the recount. A free deletes the
// block's bytes, child list, queue entry, proof and count record, and gives
// each child its down, as freeing describes.
//
// cands are queue entries a scan proposed from a read snapshot. Each counts
// only while its entry is still on the queue in this transaction, since a
// Land that names a queued block deletes its entry; one that is gone the
// slice passes over. A child the slice's frees bring to zero is queued,
// never freed in the same slice: the commit between its zero and its free
// lets a Land that names it dequeue it, as a put that names a block it saw
// stored relies on, and the free's tail check reads such a Land's record.
//
// It frees up to a slice of blocks, one while a writer waits, and stops
// after a free that starts a paged release or one whose list does not fit
// the moves left. It reports how many of cands it took, a prefix, how many
// blocks it freed or parked, and whether a free took a child to zero or
// started a paged release.
func (c *Collector) freeSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, cands []queuedBlock) (int, int, bool, error) {
	// Free only in an adopted Volume with no release in progress and no
	// recount owed.
	state, err := rec.GetState(ctx)
	if err != nil || !c.freeable(state) {
		return 0, 0, false, err
	}
	if owed, err := rec.recountOwed(ctx); err != nil || owed {
		return 0, 0, false, err
	}

	// The frees give their children downs within the moves a netting slice
	// makes, one page's while a writer waits.
	f := &freeing{moves: c.share(sliceReleases)}
	frees := c.share(sliceFrees)
	if c.waiting() {
		f.moves = min(f.moves, releasePageChildren)
		frees = 1
	}

	// Free the entries still queued, in order, until the slice's frees are
	// done or a writer waits.
	took, done := 0, 0
	for ; done < frees && took < len(cands); took++ {
		if done != 0 && c.waiting() {
			break
		}
		q := cands[took]
		if q.ripe > state.GetUpCursor() {
			break
		}
		found, err := rec.tx.Exists(ctx, rec.queueKey(q.ripe, q.key))
		if err != nil {
			return 0, 0, false, err
		}
		if !found {
			continue
		}
		freed, err := c.freeBlock(ctx, rec, blocks, state, q, res, f)
		if err != nil || !freed {
			return took, done, f.gave, err
		}
		done++
		if state.GetRelease() != nil {
			return took + 1, done, f.gave, nil
		}
	}
	return took, done, f.gave, nil
}

// requeueSlice moves the parked blocks page names, read after the parked key
// position from in a read snapshot, back onto the free queue once the
// generation flipped, and reports whether the page was full and whether it
// reached the parked keys' end. It moves only a key its transaction still reads
// parked, since a Land that stored the block again since the snapshot unparked
// it. It reads only kindParked keys: a released park gave its downs already, so
// it stays for the sweep, which deletes its key and gives none. Each keeps its
// ripe sequence, which the cursor passed before the free parked it, so the free
// step that follows takes it, checking it is still at zero at that sequence,
// and drops an unmarked one, which the sweep deletes. While the generation is
// Open, or the Volume is not adopted, it moves nothing, and an Open one resets
// the requeue's position, as Collector.requeued describes.
//
// Cost: per key, one point read, and for a key still parked, its delete and
// enqueue.
func (c *Collector) requeueSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte, page *keyPage) ([]byte, bool, bool, error) {
	// Requeue only once an adopted Volume's generation flipped.
	state, err := rec.GetState(ctx)
	if err != nil {
		return from, false, false, err
	}
	if state.GetGeneration() == Generation_GENERATION_OPEN {
		return nil, false, false, nil
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return from, false, false, nil
	}

	// Move each key still parked.
	err = page.each(func(key []byte) error {
		// Skip a key a Land took since the snapshot.
		if len(key) < 8 {
			return errBadBlockKey
		}
		parked, err := rec.tx.Exists(ctx, rec.key(kindParked, key))
		if err != nil || !parked {
			return err
		}
		if err := rec.tx.Delete(ctx, rec.key(kindParked, key)); err != nil {
			return err
		}
		if err := rec.enqueue(ctx, binary.BigEndian.Uint64(key), key[8:]); err != nil {
			return err
		}
		res.Requeued++
		return nil
	})
	if err != nil {
		return from, false, false, err
	}
	return page.resume(from), page.full, !page.full, nil
}

// freeable reports whether the free may run under state: the Volume is
// adopted, Grace has passed since the adoption's mark, and no paged release
// is in progress, whose unmoved children still count their freed parent.
// The adoption settles a block no holder references at zero with no down behind
// it, so the wait gives a reader that saw it named before the mark the
// Grace a down would have, as Adopt describes.
func (c *Collector) freeable(state *LivenessState) bool {
	graced := c.opts.Now().UnixMilli()-state.GetAdoptAtUnixMilli() >= c.opts.Grace.Milliseconds()
	return state.GetAdoption() == Adoption_ADOPTION_ADOPTED && graced && state.GetRelease() == nil
}

// freeBlock frees one block at zero, q, ripe at q.ripe: its bytes, child
// list, queue entry, proof and count record, and gives each child its down
// through f. The caller read q's entry in this transaction. It deletes
// only the queue entry when the entry is stale: the block has no child list
// or no zero record at its ripe sequence, or it is unmarked, which the sweep
// deletes. While the generation is Open it deletes nothing,
// and parks the block instead, as invariants 9 and 10 describe: a park
// gives the children their downs as a free does, unless ParkKeepsHolds
// keeps them. When the list does not fit the moves f has left after an
// earlier free, it changes nothing and reports false, so the slice stops
// before it.
//
// freeBlock is the one path that deletes a block the counts prove dead. It
// keeps the invariants Collector lists: hold until free, since the children
// lose the block only here, in the transaction that deletes its list; counts
// exact, since the block's zero record at q's ripe sequence is read here
// and each child moves down exactly once; no late namer, since a Land that
// held the block since it ripened deleted its queue entry, and the caller
// read the entry in this transaction; and no freed
// read, since the up cursor reached the ripe sequence, which it checks
// here, so every up before it netted.
func (c *Collector) freeBlock(
	ctx context.Context,
	rec *Records,
	blocks block.StoreOps,
	state *LivenessState,
	q queuedBlock,
	res *CollectResult,
	f *freeing,
) (bool, error) {
	// Check the block is ripe.
	ripe, key := q.ripe, q.key
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return false, err
	}
	if ripe > state.GetUpCursor() {
		return false, errors.Errorf("free of a block ripe at %d past the up cursor %d", ripe, state.GetUpCursor())
	}

	// Check it is listed.
	entry := rec.queueKey(ripe, key)
	dequeue := func() error { return rec.tx.Delete(ctx, entry) }
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil {
		return false, err
	}
	if !listed {
		return true, dequeue()
	}

	// Check its count. The netting moves a block's queue entry whenever it
	// changes the block's ripe sequence, so at differs from ripe only when
	// damage left the records apart, and the block is kept.
	holders, at, err := rec.GetHolders(ctx, ref)
	if err != nil {
		return false, err
	}
	if holders != 0 || at != ripe {
		return true, dequeue()
	}

	// Park it while the generation is Open, in the run, keeping its holds
	// only as ParkKeepsHolds asks, and leave an unmarked one to the sweep.
	park := state.GetGeneration() == Generation_GENERATION_OPEN
	switch {
	case park && c.opts.ParkKeepsHolds:
		res.Parked++
		return true, rec.park(ctx, entry, rec.parkedKey(ripe, key), key)
	case state.GetGeneration() == Generation_GENERATION_SWEEPING:
		unmarked, err := rec.Unmarked(ctx, key)
		if err != nil {
			return false, err
		}
		if unmarked {
			return true, dequeue()
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
		if err := rec.park(ctx, entry, rec.releasedKey(ripe, key), key); err != nil {
			return false, err
		}
		return true, f.give(ctx, c, rec, state, res, key, children)
	}

	// Free its bytes and records.
	if err := dequeue(); err != nil {
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
	// moves is the slice's budget of moved children, and moved is the
	// children its frees moved.
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
