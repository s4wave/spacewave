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

// cleanShare is the most of the writer lock's time the cleaner takes: after
// holding it for d, the cleaner waits at least d*(cleanShare-1).
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

// cleanMinSlack is the least slack, in bytes, worth cleaning. Tests lower it
// so the cleaner runs on small files.
var cleanMinSlack int64 = 4 << 20

// cleaner empties value chunks in the background: it moves a victim chunk's
// live values into the moved stream, so the chunk frees at release and the
// file shrinks as free chunks reach its end. It reads the victim on a
// snapshot without the writer lock, and takes the lock only for short
// commits while no writer holds or wants it.
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
func (c *cleaner) step(ctx context.Context) (time.Duration, error) {
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

	// Pick the victim and pin the state that holds it.
	least := cleanMinSlack
	if c.sync {
		least = 0
	}
	chunk, live, ok := w.sp.victim(db.cur.Load().seq, least, c.stuck)
	var placed uint64
	var st *state
	var stripe int
	if ok {
		placed = w.sp.table[chunk].seq
		st, stripe = db.acquire()
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
	moves, err := c.read(st, chunk, live)
	db.release(st, stripe)
	if err != nil {
		return 0, err
	}

	// Pass over a chunk holding values the cleaner cannot move.
	moved := 0
	for _, m := range moves {
		moved += len(m.data)
	}
	if moved < int(live) {
		if c.stuck == nil {
			c.stuck = make(map[uint64]uint64)
		}
		c.stuck[chunk] = placed
	}
	c.moves = moves
	return 0, nil
}

// read reads value chunk chunk on the snapshot st and returns the values in
// it st still refers to, up to live bytes of them. Entries run from
// the chunk's start; the walk ends at a zero length, an entry that overruns
// the chunk, or once it has found the live bytes, so bytes past the last
// entry, which a reused chunk may still hold, are never trusted. A value
// whose bytes fail its checksum stays where it is, so the move never gives
// damaged bytes a new checksum.
func (c *cleaner) read(st *state, chunk uint64, live uint32) ([]move, error) {
	// Read the chunk.
	db := c.db
	buf := make([]byte, chunkSize)
	base := chunk * chunkSize
	if _, err := db.s.ReadAt(buf, fileOff(base)); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	// Walk its entries, keeping the values the snapshot refers to.
	var moves []move
	var found uint32
	for p := 0; found < live; {
		key, data, next := valueEntry(buf, p)
		if next == 0 {
			break
		}
		p = next
		v, ok, err := db.lookup(st, key)
		if err != nil {
			return nil, err
		}
		if !ok || !v.isRef || v.ref.off != base+uint64(p-len(data)) || int(v.ref.n) != len(data) { // #nosec G115 -- p is within the chunk.
			continue
		}
		found += v.ref.n
		if checksum(data) != v.ref.crc {
			continue
		}
		moves = append(moves, move{key: bytes.Clone(key), val: v, data: data})
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
// keeping those whose key holds the reference the read saw, and returns
// the wait that keeps the cleaner within its lock share and its rate.
func (c *cleaner) commit(ctx context.Context) (wait time.Duration, err error) {
	// Take the writer lock only when idle.
	db, w := c.db, c.db.w
	if held, err := c.lock(ctx); !held {
		return cleanBusy, err
	}
	start := time.Now()
	defer func() { w.unlock(err) }()

	// Take a batch.
	n, size := 0, 0
	for n < len(c.moves) && (n == 0 || size+len(c.moves[n].data) <= cleanBatch) {
		size += len(c.moves[n].data)
		n++
	}
	batch := c.moves[:n]
	c.moves = c.moves[n:]

	// Keep the values still current, and move them.
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
	if len(changes) != 0 {
		slices.SortFunc(changes, func(a, b tentry) int { return bytes.Compare(a.key, b.key) })
		if err := w.commit(ctx, st, changes, true, true); err != nil {
			return 0, err
		}
		c.moved = db.cur.Load().seq
	}

	// Wait so the cleaner's lock time stays a share of the writer's, and its
	// bytes within the rate.
	d := time.Since(start)
	return max(d*(cleanShare-1), time.Duration(moved)*time.Second/time.Duration(db.opts.RelocateBudget)), nil
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
