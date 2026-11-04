package s4db

import (
	"bytes"
	"context"
	"time"
)

// compactVisits bounds the entries one background compaction step reads.
const compactVisits = 4096

// compactStepBytes bounds the value bytes one background step moves.
const compactStepBytes = 1 << 20

// compactRate bounds the value bytes background compaction moves a second.
const compactRate = 8 << 20

// compactShare is the most of the writer lock's time background compaction
// takes: after holding it for d, a step waits at least d*(compactShare-1).
const compactShare = 10

// compactIdle is how long background compaction waits before looking again
// when the file has no slack or a writer holds the lock.
const compactIdle = time.Second

// compactBusy is how long a step waits when a writer holds or wants the lock.
const compactBusy = 5 * time.Millisecond

// compactRest is how long background compaction first waits after a full pass
// over the keys that neither moved nor took off the file's end the least slack
// worth compacting. Each such pass in a row doubles the wait up to
// compactRestMax, since a pass reads the whole index.
const compactRest = time.Minute

// compactRestMax bounds the wait after passes that moved little.
const compactRestMax = 16 * time.Minute

// compactMinSlack is the least free space, in pages, worth compacting away.
// Tests lower it so the compactor runs on small files.
var compactMinSlack uint64 = 64 << 20 / pageSize

// compactor moves values toward the start of the file in the background, so
// the file shrinks as the space they leave reaches its end. It reads values
// on a snapshot without the writer lock, and takes the lock only for a short
// commit when no writer holds or waits for it.
type compactor struct {
	// db is the database.
	db *DB
	// cursor is the key the next step reads from, or nil at the start.
	cursor []byte
	// goal is the end the last plan aimed at, or 0 without one.
	goal uint64
	// sparse reports that at the last plan a quarter of the value pages'
	// bytes were free.
	sparse bool
	// passFound counts the values found to move since the cursor last
	// wrapped.
	passFound int
	// passMoved counts the bytes rewritten since the cursor last wrapped.
	passMoved int64
	// end is the file end, in pages, at the last plan.
	end uint64
	// passEnd is the end at the last plan before the last pass ended, or 0
	// before the first.
	passEnd uint64
	// rest is the wait after the last pass, or 0 after one that moved
	// enough.
	rest time.Duration
	// touched holds the inner pages past the goal a move rewrites in this
	// pass.
	touched map[uint64]bool
	// flush asks the next plan to checkpoint, so the index pages the last
	// pass rewrote move below the goal.
	flush bool
}

// move is a value a step may move, or an index page it may rewrite.
type move struct {
	// key is the key.
	key []byte
	// val is the value the step read.
	val value
	// page is the index page past the goal that rewriting key rewrites, or 0
	// for a value move.
	page uint64
	// data is the value.
	data []byte
}

// run compacts until ctx ends, pacing each step by the lock time it took and
// the bytes it moved. A failed step backs off; the writer rebuilds the space
// the next time it takes the lock.
func (c *compactor) run(ctx context.Context) error {
	backoff := compactIdle
	for {
		// Take a step, then wait as it asks, longer after a failure.
		wait, err := c.step(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			wait, backoff = backoff, min(2*backoff, time.Minute)
		} else {
			backoff = compactIdle
		}

		// Sleep, or end with ctx.
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return context.Canceled
		case <-t.C:
		}
	}
}

// step moves one batch of values and returns how long to wait before the
// next.
func (c *compactor) step(ctx context.Context) (time.Duration, error) {
	// Decide whether the file has slack worth moving away.
	db := c.db
	ok, busy, err := c.plan(ctx)
	if busy {
		return compactBusy, nil
	}
	if err != nil || !ok {
		return compactIdle, err
	}

	// On one snapshot, list the next values, keep those the space would
	// place lower, and read them. The snapshot keeps their pages readable.
	start, from := time.Now(), c.cursor
	st, stripe := db.acquire()
	refs, wrapped, err := c.scan(ctx, st)
	var moves []move
	if err == nil {
		moves, busy, err = c.choose(ctx, refs)
	}
	if err == nil && !busy {
		err = c.read(moves)
	}
	db.release(st, stripe)
	if err != nil {
		return compactBusy, err
	}

	// A writer took the lock first: scan the same keys again shortly.
	if busy {
		c.cursor = from
		return compactBusy, nil
	}

	// Pace a scan that found nothing like a commit, so scanning takes a
	// share of a core, and rest after a pass that moved little.
	c.passFound += len(moves)
	if len(moves) == 0 {
		wait := max(compactBusy, time.Since(start)*(compactShare-1))
		if wrapped {
			wait = max(wait, c.endPass())
		}
		return wait, nil
	}

	// Commit the moves under the writer lock.
	start = time.Now()
	held, moved, err := c.commit(ctx, moves)
	if err != nil {
		return 0, err
	}
	c.passMoved += int64(moved)
	if !held {
		return compactBusy, nil
	}

	// Wait so the step's lock time stays a share of the writer's, and its
	// bytes within the rate, and rest after a pass that moved little.
	d := time.Since(start)
	wait := max(d*(compactShare-1), time.Duration(moved)*time.Second/compactRate)
	if wrapped {
		wait = max(wait, c.endPass())
	}
	return wait, nil
}

// endPass ends a pass over the keys and returns how long to rest: nothing
// after a pass that moved, or took off the file's end since the last pass
// ended, at least the least slack worth compacting, else a wait that doubles
// with each such pass in a row. The end falls mostly at the checkpoint after
// a pass, so a pass counts the fall the one before it caused. A pass that
// found values to move checkpoints at the next plan.
func (c *compactor) endPass() time.Duration {
	// Ask for the checkpoint, and start the next pass's counts.
	c.flush = c.passFound > 0
	moved := c.passMoved
	lowered := c.passEnd > c.end && c.passEnd-c.end >= compactMinSlack
	c.passFound, c.passMoved, c.touched, c.passEnd = 0, 0, nil, c.end

	// Rest longer after each pass that did little.
	if moved >= pageOff(compactMinSlack) || lowered {
		c.rest = 0
		return 0
	}
	c.rest = min(max(2*c.rest, compactRest), compactRestMax)
	return c.rest
}

// plan decides under the writer lock whether the file holds slack worth
// compacting: free runs past an eighth of the pages in use, or sparse value
// pages. It sets the goal end that allocation then stays below, and
// continues the log below the goal when the log lies past it. busy reports
// that a writer held or wanted the lock.
func (c *compactor) plan(ctx context.Context) (ok, busy bool, err error) {
	// Take the writer lock only when no writer holds or wants it.
	w := c.db.w
	if !w.lockIdle(ctx) {
		return false, true, nil
	}
	defer func() { w.unlock(err) }()

	// Note the end, so the pass's end can tell whether it fell.
	sp := w.sp
	c.end = sp.end

	// Aim at the pages in use and an eighth more, once the free runs hold
	// that much and the minimum.
	inUse := sp.end - sp.freeCount
	goal := inUse + inUse/8
	live, held := sp.stats()
	sparse := held-live >= max(1<<20, held/4)
	c.sparse = sparse
	if sp.freeCount < compactMinSlack || goal >= sp.end {
		sp.goal, c.goal = 0, 0
		return sparse, false, nil
	}
	sp.goal, c.goal = goal, goal
	sp.moveCut()

	// Checkpoint after a pass that rewrote keys, so the index pages they
	// lie in move below the goal.
	if c.flush {
		c.flush = false
		w.startCheckpoint()
	}

	// Continue the log below the goal.
	if c.db.cur.Load().pos/pageSize >= goal {
		if err = w.moveLog(ctx); err != nil {
			return false, false, err
		}
	}
	return true, false, nil
}

// scan lists, on st's index, the values stored outside the index among the
// next compactVisits entries from the cursor, and one key in each index page
// past the goal, and advances the cursor. wrapped reports that the scan
// reached the last key, so the next starts over. A key changed since the
// last checkpoint lists its current value; keys added since reach the index
// at the next checkpoint.
func (c *compactor) scan(ctx context.Context, st *state) (refs []move, wrapped bool, err error) {
	// Start at the cursor. The pass reads every leaf once, so it leaves them
	// out of the cache.
	cur := cursor{p: c.db.p, root: st.root, scan: true}
	cur.seek(c.cursor, false)

	// List the values outside the index, and the first key reached in each
	// index page past the goal.
	var leaf *node
	for visits := compactVisits; cur.valid() && visits > 0; cur.step(false) {
		visits--
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		m := move{val: cur.value()}
		if n := cur.path[len(cur.path)-1].n; n != leaf {
			leaf = n
			m.page = c.pastGoal(&cur)
		}

		// Take the key's value from st, not the index: a commit since the
		// checkpoint may have changed or deleted it, and freed the space the
		// index's value lies in.
		if (m.page != 0 || m.val.isRef) && st.filter.has(cur.key()) {
			if it, ok := st.overlay.Get(overlayProbe(cur.key())); ok {
				if it.e.del {
					continue
				}
				m.val = it.e.val
			}
		}
		if m.page != 0 || m.val.isRef {
			m.key = bytes.Clone(cur.key())
			m.val.inline = bytes.Clone(m.val.inline)
			refs = append(refs, m)
		}
	}
	if cur.err != nil {
		return nil, false, cur.err
	}

	// Save where the next step resumes.
	c.cursor = nil
	if cur.valid() {
		c.cursor = bytes.Clone(cur.key())
	}
	return refs, c.cursor == nil, nil
}

// pastGoal returns the highest page on the cursor's path past the goal that
// no move in this pass rewrites yet, or 0. Rewriting a key rewrites every
// page on its path.
func (c *compactor) pastGoal(cur *cursor) uint64 {
	// Find the highest untouched page past the goal; a leaf always counts.
	if c.goal == 0 {
		return 0
	}
	var found uint64
	page := cur.root
	for i, f := range cur.path {
		if page >= c.goal && page > found && (i == len(cur.path)-1 || !c.touched[page]) {
			found = page
		}
		if !f.n.leaf {
			page = f.n.kid(f.i)
		}
	}
	if found == 0 {
		return 0
	}

	// Mark the inner pages the move rewrites.
	if c.touched == nil {
		c.touched = make(map[uint64]bool)
	}
	page = cur.root
	for _, f := range cur.path {
		if !f.n.leaf {
			c.touched[page] = true
			page = f.n.kid(f.i)
		}
	}
	return found
}

// choose keeps, under the writer lock, up to compactStepBytes of the values
// that would move lower: those past the goal or in a page under half full,
// for which free space below would take them, and the keys whose index page
// would move lower. The cursor returns to the
// first value left over. busy reports that a writer held or wanted the lock.
func (c *compactor) choose(ctx context.Context, refs []move) (moves []move, busy bool, err error) {
	// Take the writer lock only when idle.
	w := c.db.w
	if len(refs) == 0 {
		return nil, false, nil
	}
	if !w.lockIdle(ctx) {
		return nil, true, nil
	}
	defer func() { w.unlock(nil) }()

	// Keep the values the space would place lower.
	sp := w.sp
	var n int
	for _, m := range refs {
		if n >= compactStepBytes {
			c.cursor = m.key
			break
		}
		if c.lowers(sp, m) {
			moves = append(moves, m)
			n += int(m.val.ref.n) + pageSize
		}
	}
	return moves, false, nil
}

// lowers reports whether m's index page would move lower, or its value lies
// past the goal or in a page under half full and free space below would take
// it.
func (c *compactor) lowers(sp *space, m move) bool {
	// An index page moves when a free page below would take it.
	if m.page != 0 && sp.lower(pageSize, m.page) {
		return true
	}

	// While a quarter of the value space is free inside value pages, a
	// value in a sparse page moves wherever placement puts it, since no
	// free run below may take it until such pages empty.
	if !m.val.isRef {
		return false
	}
	ref := m.val.ref
	p := ref.off / pageSize
	sparse := sp.liveAt(p) < pageSize/2
	if sparse && c.sparse {
		return true
	}

	// Otherwise a value moves when it lies past the goal or in a sparse
	// page, and free space below would take it.
	past := sp.goal != 0 && p >= sp.goal
	return (past || sparse) && sp.lower(int(ref.n), p)
}

// read reads the moves' values.
func (c *compactor) read(moves []move) error {
	for i := range moves {
		if !moves[i].val.isRef {
			moves[i].data = moves[i].val.inline
			continue
		}
		data, err := c.db.p.readValue(moves[i].val)
		if err != nil {
			return err
		}
		moves[i].data = data
	}
	return nil
}

// commit rewrites the moves whose keys still hold the value read and that
// still would move lower, under the writer lock when no writer holds or
// wants it. held reports whether it took the lock; moved counts the bytes
// rewritten, a page for each index page.
func (c *compactor) commit(ctx context.Context, moves []move) (held bool, moved int, err error) {
	// Take the writer lock only when idle.
	db, w := c.db, c.db.w
	if !w.lockIdle(ctx) {
		return false, 0, nil
	}
	defer func() { w.unlock(err) }()

	// Keep the values that did not change since the scan.
	sp, st := w.sp, db.cur.Load()
	var changes []tentry
	for _, m := range moves {
		cur, found, err := db.lookup(st, m.key)
		if err != nil {
			return true, 0, err
		}
		if !found || !sameValue(cur, m.val) || !c.lowers(sp, m) {
			continue
		}
		changes = append(changes, tentry{key: m.key, data: m.data})
		moved += len(m.data)
		if m.page != 0 {
			moved += pageSize
		}
	}
	if len(changes) == 0 {
		return true, 0, nil
	}

	// Rewrite them in a new extent: placement prefers the lowest free pages.
	sp.closeOpen(st.seq)
	return true, moved, w.commit(ctx, st, changes, true, true)
}

// sameValue reports whether a and b hold the same value.
func sameValue(a, b value) bool {
	if a.isRef || b.isRef {
		return a.isRef == b.isRef && a.ref == b.ref
	}
	return bytes.Equal(a.inline, b.inline)
}

// lockIdle takes the writer lock when no writer of this handle holds or
// waits for it, and brings the writer up to date. It reports whether it took
// the lock.
func (w *writer) lockIdle(ctx context.Context) bool {
	// Leave the lock to waiting writers.
	if w.waiting() {
		return false
	}
	ok, err := w.tryLock()
	if err != nil || !ok {
		return false
	}

	// Catch up with other processes, and step aside for a writer that
	// arrived meanwhile.
	if err := w.catchUp(ctx); err != nil {
		w.unlock(err)
		return false
	}
	if w.waiting() || w.sp == nil {
		w.unlock(nil)
		return false
	}
	return true
}
