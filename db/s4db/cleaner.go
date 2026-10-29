package s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"slices"
	"time"

	"github.com/pkg/errors"
)

// cleanShare is the most of the writer lock's time the urgent cleaner takes:
// after holding it for d, it waits at least d*(cleanShare-1).
const cleanShare = 10

// cleanBusy is how long the cleaner waits when a writer holds or wants the
// lock.
const cleanBusy = 5 * time.Millisecond

// cleanIdle is the first wait after the cleaner found nothing to clean. Each
// such look in a row doubles it, up to cleanRestMax.
const cleanIdle = time.Second

// cleanRestMax bounds the wait between looks that find nothing to clean.
const cleanRestMax = time.Minute

// cleanBatch bounds the value bytes one cleaner commit moves.
const cleanBatch = 256 << 10

// evacRounds bounds the rounds of moves the cleaner makes to empty one index
// chunk. A round moves a value under each page the chunk still holds, and a
// checkpoint then writes those pages elsewhere; pages a checkpoint was
// writing meanwhile need another round.
const evacRounds = 3

// cleanMinSlack is the least slack, in bytes, worth cleaning. Tests lower it
// so the cleaner runs on small files.
var cleanMinSlack int64 = 4 << 20

// cleaner empties chunks in the background: it moves a victim value chunk's
// live values into the moved stream, so the chunk frees at release and the
// file shrinks as free chunks reach its end. It empties an index chunk by
// keeping new pages out of it and moving a value under each of its pages, so
// the next checkpoint rewrites the paths to them elsewhere and releases the
// pages. It reads the victim on a snapshot without the writer lock, and
// takes the lock only for short commits while no writer holds or wants it.
//
// The snapshot is held from the victim's choice through its read, and no
// longer, so it delays no release across the paced commits. A chunk with live
// bytes in the snapshot stays pending until a release no snapshot blocks, so
// the bytes the read sees are the snapshot's values. A move commits only
// while its key holds the same reference, so the copy matches the current
// reference's checksum even if the chunk was freed and reused meanwhile.
type cleaner struct {
	// db is the database.
	db *DB
	// moves are the victim's live values left to commit.
	moves []move
	// urgent is set when the slack passed the urgent bound as the moves'
	// victim was chosen: the moves skip the quiet gate and read at normal
	// priority, taking at most cleanShare of the writer lock.
	urgent bool
	// moved is the last commit of the cleaner's moves.
	moved uint64
	// rest is the wait after the last look that found nothing to clean.
	rest time.Duration
	// stuck holds the chunks a read found damaged values in, with the seq
	// the chunk held then, so the cleaner passes over them until they take
	// new values.
	stuck map[uint64]uint64
	// sync makes the cleaner wait for the writer lock and clean small files
	// too, for Compact.
	sync bool
	// evac is the index chunk the cleaner empties, or zero for none.
	evac uint64
	// rounds counts the rounds of moves made to empty evac.
	rounds int
	// settled is the last move Compact's cleaner freed the space of.
	settled uint64
	// idle reads victims for the background cleaner; nil reads them on the
	// cleaner's goroutine.
	idle *idleThread
}

// scan is what the cleaner reads of a victim chunk without a lock: a value
// chunk's entries or an index chunk's tree pages.
type scan struct {
	// entries are a value chunk's entries from its start.
	entries []chunkEntry
	// pages are an index chunk's tree pages among its used pages.
	pages []*node
}

// chunkEntry is a value entry read from a value chunk.
type chunkEntry struct {
	// key is the entry's key.
	key []byte
	// data is the entry's value bytes.
	data []byte
	// off is the file offset of data.
	off uint64
	// crc is the checksum of data.
	crc uint32
}

// move is a live value the cleaner moves.
type move struct {
	// key is the key.
	key []byte
	// val is the value the snapshot holds.
	val value
	// data is the value's bytes.
	data []byte
}

// run cleans until ctx ends. A failed step drops its victim and backs off;
// the writer rebuilds the space the next time it takes the lock.
func (c *cleaner) run(ctx context.Context) error {
	c.idle = newIdleThread(ctx)
	backoff := cleanIdle
	for {
		// Take a step, then wait as it asks, longer after a failure.
		wait, err := c.step(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			c.moves = nil
			wait, backoff = backoff, min(2*backoff, time.Minute)
		} else {
			backoff = cleanIdle
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

// step chooses and reads a victim when none is pending, else commits the
// next batch of its moves, and returns how long to wait before the next.
// The background cleaner steps only in a quiet spell after the foreground
// commits, unless the last victim was urgent.
func (c *cleaner) step(ctx context.Context) (time.Duration, error) {
	if !c.sync && !c.urgent {
		if wait := c.db.w.fg.wait(); wait > 0 {
			return wait, nil
		}
	}
	if len(c.moves) != 0 {
		return c.commit(ctx)
	}
	return c.choose(ctx)
}

// choose picks a victim under the writer lock and reads its live values on
// the state the space reflects.
func (c *cleaner) choose(ctx context.Context) (time.Duration, error) {
	// Take the lock while no writer holds or wants it, and free what
	// earlier moves emptied.
	db, w := c.db, c.db.w
	if held, err := c.lock(ctx); !held {
		return cleanBusy, err
	}
	if err := c.release(ctx); err != nil {
		w.unlock(err)
		return 0, err
	}

	// Go on emptying an index chunk, or pick a victim.
	victim, ok, wait := c.continueEvac()
	if wait {
		return c.await(ctx)
	}
	urgent := false
	if !ok {
		least := cleanMinSlack
		if c.sync {
			least = 0
		}
		victim, urgent, ok = w.sp.victim(db.cur.Load().seq, least, c.stuck)
	}

	// Pin the state that holds the victim, keeping new index pages out of
	// an index chunk.
	var e chunk
	var st *state
	var stripe int
	if ok {
		e = w.sp.table[victim]
		if e.kind == chunkIndex && c.evac != victim {
			c.evac, c.rounds, w.sp.evac = victim, 0, victim
		}
		st, stripe = db.acquire()
	} else if c.sync && c.settled < c.moved {
		// Free what the moves emptied and look again: the chunks at the
		// end can now move down into it.
		return c.await(ctx)
	} else if _, logged, _ := db.flush.marks(); logged < c.moved && !c.sync {
		// A commit's frees release only under a later durable mark, so
		// once the moves end a checkpoint marks the last of them.
		w.startCheckpoint()
	}
	w.unlock(nil)

	// Rest longer after each look in a row that finds nothing.
	if !ok {
		c.rest = min(max(2*c.rest, cleanIdle), cleanRestMax)
		return c.rest, nil
	}
	c.rest = 0
	moves, err := c.read(ctx, st, victim, &e, urgent)
	db.release(st, stripe)
	if err != nil {
		return 0, err
	}

	// Pass over a value chunk holding values the cleaner cannot move.
	moved := 0
	for _, m := range moves {
		moved += len(m.data)
	}
	if e.kind == chunkValue && moved < int(e.live) {
		c.pass(victim, e.stamp())
	}
	c.moves, c.urgent = moves, urgent
	return 0, nil
}

// continueEvac reports whether the index chunk being emptied takes another
// round of moves, or whether to await the checkpoint that rewrites the pages
// the last round dirtied. A chunk still holding pages after evacRounds is
// passed over until its pages change. The caller holds the writer lock.
func (c *cleaner) continueEvac() (chunk uint64, again, wait bool) {
	// Drop the chunk once it is free.
	sp := c.db.w.sp
	if c.evac == 0 {
		return 0, false, false
	}
	if c.evac >= uint64(len(sp.table)) || sp.table[c.evac].kind != chunkIndex {
		c.evac = 0
		return 0, false, false
	}
	sp.evac = c.evac

	// Wait for the checkpoint after the last moves.
	if _, _, ckpt := c.db.flush.marks(); ckpt < c.moved || c.sync && c.settled < c.moved {
		return 0, false, true
	}

	// Move again, or give up on the chunk.
	if c.rounds++; c.rounds > evacRounds {
		c.pass(c.evac, sp.table[c.evac].stamp())
		c.evac, sp.evac = 0, 0
		return 0, false, false
	}
	return c.evac, true, false
}

// await gives up the writer lock until a checkpoint follows the last moves.
// The background cleaner starts one and rests. Compact's cleaner waits for
// any background build, whose snapshot keeps the moved chunks from freeing,
// then checkpoints and releases until what the moves freed is free.
func (c *cleaner) await(ctx context.Context) (time.Duration, error) {
	// Start a checkpoint in the background.
	db, w := c.db, c.db.w
	if !c.sync {
		w.startCheckpoint()
		w.unlock(nil)
		return cleanIdle, nil
	}

	// Settle at once for Compact.
	w.unlock(nil)
	if err := db.lockIdle(ctx); err != nil {
		return 0, err
	}
	c.settled = c.moved
	err := w.compact(ctx)
	w.unlock(err)
	return 0, err
}

// pass passes over chunk until its stamp changes from stamp.
func (c *cleaner) pass(chunk, stamp uint64) {
	if c.stuck == nil {
		c.stuck = make(map[uint64]uint64)
	}
	c.stuck[chunk] = stamp
}

// read reads victim chunk e on the snapshot st and returns the moves that
// empty it. It scans the chunk on the idle thread, unless the slack is urgent
// or the cleaner has none, and looks the keys up on the caller's goroutine,
// since a lookup takes the cache's locks.
func (c *cleaner) read(ctx context.Context, st *state, victim uint64, e *chunk, urgent bool) ([]move, error) {
	// Scan the chunk.
	var sc scan
	var err error
	scanChunk := func() { sc, err = c.scan(victim, e) }
	if c.idle == nil || urgent {
		scanChunk()
	} else if derr := c.idle.do(ctx, scanChunk); derr != nil {
		return nil, derr
	}
	if err != nil {
		return nil, err
	}

	// Take the moves.
	if e.kind == chunkIndex {
		return c.readIndex(st, sc.pages)
	}
	return c.readValues(st, sc.entries, e.live)
}

// scan reads chunk e at victim and parses it: a value chunk into its entries
// from its start, each with its checksum, and an index chunk into the tree
// pages among its used pages. The entries end at a zero length or an entry
// that overruns the chunk. Scan takes no lock.
func (c *cleaner) scan(victim uint64, e *chunk) (scan, error) {
	// Read the chunk.
	buf := make([]byte, chunkSize)
	base := victim * chunkSize
	if _, err := c.db.s.ReadAt(buf, fileOff(base)); err != nil && !errors.Is(err, io.EOF) {
		return scan{}, err
	}

	// Decode the used tree pages of an index chunk.
	var sc scan
	if e.kind == chunkIndex {
		for p := range uint64(chunkPages) {
			if e.used[p/64]&(1<<(p%64)) == 0 {
				continue
			}
			if n, err := decodeNode(buf[p*pageSize : (p+1)*pageSize]); err == nil {
				sc.pages = append(sc.pages, n)
			}
		}
		return sc, nil
	}

	// Walk a value chunk's entries.
	for p := 0; ; {
		key, data, next := valueEntry(buf, p)
		if next == 0 {
			return sc, nil
		}
		p = next
		off := base + uint64(p-len(data)) // #nosec G115 -- p is within the chunk.
		sc.entries = append(sc.entries, chunkEntry{key: key, data: data, off: off, crc: checksum(data)})
	}
}

// readIndex returns, for each tree page of an index chunk, one value under it
// that st still holds: the smallest in a leaf, or in the leftmost leaf under
// an inner page. Moving it dirties the path through the page, so the next
// checkpoint writes that path elsewhere. Other pages, such as a space
// record's, free with the next checkpoint, so when the chunk holds no tree
// page it returns one value from the tree's leftmost leaf, whose move brings
// that checkpoint.
func (c *cleaner) readIndex(st *state, pages []*node) ([]move, error) {
	// Take a value under each tree page.
	db := c.db
	var moves []move
	seen := make(map[string]bool)
	for _, n := range pages {
		m, ok, err := c.under(st, n, seen)
		if err != nil {
			return nil, err
		}
		if ok {
			moves = append(moves, m)
		}
	}

	// Bring a checkpoint for a chunk of other pages.
	if len(moves) != 0 || st.root == 0 {
		return moves, nil
	}
	n, err := db.p.node(st.root)
	if err != nil {
		return nil, err
	}
	m, ok, err := c.under(st, n, seen)
	if err != nil || !ok {
		return nil, err
	}
	return []move{m}, nil
}

// under returns the move of the smallest value in the leftmost leaf under
// tree page n, if st still holds it and its key is not in seen, which it
// joins.
func (c *cleaner) under(st *state, n *node, seen map[string]bool) (move, bool, error) {
	// Find the leftmost leaf and its smallest value. A page a checkpoint
	// still holds keeps its kids, so the walk reads only used pages.
	db := c.db
	var err error
	for !n.leaf && n.count() != 0 {
		if n, err = db.p.node(n.kid(0)); err != nil {
			return move{}, false, nil
		}
	}
	if n.count() == 0 {
		return move{}, false, nil
	}
	i := smallest(n)
	key := n.key(i)
	if seen[string(key)] {
		return move{}, false, nil
	}
	seen[string(key)] = true

	// Keep it while st holds it and its bytes read back whole.
	v, ok, err := db.lookup(st, key)
	if err != nil || !ok || !sameValue(v, n.val(i)) {
		return move{}, false, err
	}
	data, err := db.p.readValue(v)
	if err != nil {
		return move{}, false, nil
	}
	return move{key: bytes.Clone(key), val: v, data: bytes.Clone(data)}, true, nil
}

// smallest returns the entry of leaf n with the smallest value.
func smallest(n *node) int {
	best, size := 0, -1
	for i := range n.count() {
		if s := n.val(i).size(); size < 0 || s < size {
			best, size = i, s
		}
	}
	return best
}

// readValues returns the entries of a value chunk that st still refers
// to, up to live bytes of them. The walk ends once it has found the live
// bytes, so bytes past the last entry, which a reused chunk may still hold,
// are never trusted. A value whose bytes fail its checksum stays where it
// is, so the move never gives damaged bytes a new checksum.
func (c *cleaner) readValues(st *state, entries []chunkEntry, live uint32) ([]move, error) {
	var moves []move
	var found uint32
	for _, en := range entries {
		if found >= live {
			break
		}
		v, ok, err := c.db.lookup(st, en.key)
		if err != nil {
			return nil, err
		}
		if !ok || !v.isRef || v.ref.off != en.off || int(v.ref.n) != len(en.data) {
			continue
		}
		found += v.ref.n
		if en.crc != v.ref.crc {
			continue
		}
		moves = append(moves, move{key: bytes.Clone(en.key), val: v, data: en.data})
	}
	return moves, nil
}

// valueEntry parses the entry at p of a value chunk's bytes b, returning its
// key, its value, and the offset after it, or a zero offset when no whole
// entry starts at p.
func valueEntry(b []byte, p int) (key, data []byte, next int) {
	// Read the key after its length plus one.
	k, l := binary.Uvarint(b[p:])
	if l <= 0 || k == 0 || k-1 > uint64(len(b)-p-l) { // #nosec G115 -- p is within b.
		return nil, nil, 0
	}
	p += l
	key, p = b[p:p+int(k-1)], p+int(k-1) // #nosec G115 -- bounded by len(b).

	// Read the value after its length.
	n, l := binary.Uvarint(b[p:])
	if l <= 0 || n > uint64(len(b)-p-l) { // #nosec G115 -- p is within b.
		return nil, nil, 0
	}
	p += l
	return key, b[p : p+int(n)], p + int(n) // #nosec G115 -- bounded by len(b).
}

// commit moves the next batch of the victim's values under the writer lock,
// keeping those whose key holds the reference the read saw, and returns the
// wait that keeps the cleaner within its rate and, when urgent, its lock
// share. The moves ride the next flush: flushing them here would make a
// foreground commit that arrives meanwhile wait for two flushes.
func (c *cleaner) commit(ctx context.Context) (time.Duration, error) {
	// Take the writer lock only when idle.
	db, w := c.db, c.db.w
	if held, err := c.lock(ctx); !held {
		return cleanBusy, err
	}
	start := time.Now()
	moved, err := c.commitBatch(ctx)
	held := time.Since(start)
	w.unlock(err)
	if err != nil {
		return 0, err
	}

	// Keep the bytes within the rate, and the urgent cleaner's lock time a
	// share of the writer's.
	wait := time.Duration(moved) * time.Second / time.Duration(db.opts.RelocateBudget)
	if c.urgent {
		wait = max(wait, held*(cleanShare-1))
	}
	return wait, nil
}

// commitBatch commits the next batch of moves whose keys still hold the
// values read, and returns the bytes it moved. The caller holds the writer
// lock.
func (c *cleaner) commitBatch(ctx context.Context) (int, error) {
	// Take a batch.
	n, size := 0, 0
	for n < len(c.moves) && (n == 0 || size+len(c.moves[n].data) <= cleanBatch) {
		size += len(c.moves[n].data)
		n++
	}
	batch := c.moves[:n]
	c.moves = c.moves[n:]

	// Keep the values still current, and move them.
	db := c.db
	st := db.cur.Load()
	var changes []tentry
	moved := 0
	for _, m := range batch {
		cur, found, err := db.lookup(st, m.key)
		if err != nil {
			return 0, err
		}
		if found && sameValue(cur, m.val) {
			changes = append(changes, tentry{key: m.key, data: m.data})
			moved += len(m.data)
		}
	}

	// Commit them in key order.
	if len(changes) == 0 {
		return 0, nil
	}
	slices.SortFunc(changes, func(a, b tentry) int { return bytes.Compare(a.key, b.key) })
	if err := db.w.commit(ctx, st, changes, true, true); err != nil {
		return 0, err
	}
	c.moved = db.cur.Load().seq
	return moved, nil
}

// release frees the chunks no snapshot reads, so the chunks earlier moves
// emptied count as free when the next victim is chosen. Compact first
// flushes the moves, so the marks later records carry cover them. The caller
// holds the writer lock.
func (c *cleaner) release(ctx context.Context) error {
	if c.sync {
		if err := c.db.flush.syncThrough(ctx, c.db.cur.Load().seq, 0); err != nil {
			return err
		}
	}
	return c.db.w.releaseNow(ctx)
}

// lock takes the writer lock: when idle, or waiting for it when sync is set.
func (c *cleaner) lock(ctx context.Context) (bool, error) {
	if c.sync {
		err := c.db.w.lock(ctx)
		return err == nil, err
	}
	return c.db.w.lockIdle(ctx), nil
}

// clean moves values until no chunk is worth cleaning.
func (c *cleaner) clean(ctx context.Context) error {
	for {
		if _, err := c.step(ctx); err != nil {
			return err
		}
		if c.rest != 0 {
			return nil
		}
	}
}

// sameValue reports whether a and b are the same stored value.
func sameValue(a, b value) bool {
	if a.isRef || b.isRef {
		return a.isRef == b.isRef && a.ref == b.ref
	}
	return bytes.Equal(a.inline, b.inline)
}

// lockIdle takes the writer lock and catches up when no writer of this
// handle waits for it and no process holds it, reporting whether it holds
// it.
func (w *writer) lockIdle(ctx context.Context) bool {
	// Take the lock only when nobody waits.
	if w.waiting.Load() != 0 {
		return false
	}
	ok, err := w.tryLock()
	if err != nil || !ok {
		return false
	}

	// Catch up, and give the lock back to a writer that came meanwhile.
	if err := w.catchUp(ctx); err != nil {
		w.unlock(err)
		return false
	}
	if w.waiting.Load() != 0 || w.sp == nil {
		w.unlock(nil)
		return false
	}
	return true
}
