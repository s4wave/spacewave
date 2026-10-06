package s4db

import (
	"context"
	"math"
	"math/rand/v2"
	"os"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

const (
	// MaxKeySize is the longest key.
	MaxKeySize = 1024
	// MaxValueSize is the longest value, the most an extent length holds.
	MaxValueSize = math.MaxUint32
	// maxRecord is the longest commit record, the most its length field
	// holds.
	maxRecord = math.MaxUint32
)

// DB is an open database file.
type DB struct {
	// f is the database file.
	f *os.File
	// id registers f in the process's open files.
	id fileID
	// hdr is the file header.
	hdr *header
	// opts holds the options with defaults applied.
	opts Options
	// p reads index pages and values.
	p *pager
	// slot is this handle's reader slot.
	slot int
	// watch wakes the tail loop when the file changes.
	watch *watcher
	// flush shares flushes between commits.
	flush *flusher
	// w writes commits and checkpoints.
	w *writer
	// tailer runs the tail loop.
	tailer *routine.RoutineContainer
	// warmer reads the tree into the cache after open.
	warmer *routine.RoutineContainer

	// cur is the published state. Read transactions acquire it without a
	// lock.
	cur atomic.Pointer[state]

	// bcast guards the fields below, serializes overlay copies and state
	// publication, and wakes waiters when a state is published or the
	// database closes. It is held only for memory work.
	bcast broadcast.Broadcast
	// retired holds earlier states that snapshots may still read.
	retired []*state
	// closed is set once Close finishes.
	closed bool

	// pinMtx guards pinned and serializes writes to the slot.
	pinMtx csync.Mutex
	// pinned is the pin last written to the slot.
	pinned pin
}

// Open opens or creates the database at path.
func Open(path string, opts Options) (*DB, error) {
	// Open the file once per process.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G703 -- the caller chooses the path.
	if err != nil {
		return nil, err
	}
	id, err := openFiles.claim(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	opts = opts.withDefaults()
	db := &DB{
		f:      f,
		id:     id,
		opts:   opts,
		p:      &pager{r: f, cache: newCache(opts.CacheBytes)},
		tailer: routine.NewRoutineContainer(),
		warmer: routine.NewRoutineContainer(),
	}
	db.flush = newFlusher(f, &db.cur)
	db.w = &writer{db: db}

	// Recover the state, releasing everything on failure.
	if err := db.open(path); err != nil {
		if db.watch != nil {
			db.watch.close()
		}
		_ = f.Close()
		openFiles.release(id)
		return nil, err
	}

	// Follow other processes' commits and warm the cache.
	db.tailer.SetRoutine(db.tailLoop)
	db.tailer.SetContext(context.Background(), false)
	db.warmer.SetRoutine(db.warm)
	db.warmer.SetContext(context.Background(), false)
	return db, nil
}

// open creates the file if empty, takes a reader slot, and recovers state.
func (db *DB) open(path string) error {
	// Create the file under the writer lock so concurrent openers agree.
	if _, err := lock(db.f, lockWriter, true); err != nil {
		return err
	}
	err := db.w.create()
	if uerr := unlock(db.f, lockWriter); err == nil {
		err = uerr
	}
	if err != nil {
		return err
	}

	// Read the header.
	page := make([]byte, pageSize)
	if _, err := db.f.ReadAt(page, headerPage*pageSize); err != nil {
		return err
	}
	if db.hdr, err = decodeHeader(page); err != nil {
		return err
	}

	// Take a free slot, pin everything until the state is loaded, and
	// watch the file.
	if db.slot, err = takeSlot(db.f); err != nil {
		return err
	}
	if err := db.writePin(pin{}); err != nil {
		return err
	}
	if db.watch, err = newWatcher(db.f, path, db.slot); err != nil {
		return err
	}

	// Load the checkpoint and the log after it, verifying values the writer
	// had not flushed.
	sb, err := db.readSuper()
	if err != nil {
		return err
	}
	st, err := db.tail(newState(sb, db.opts.checkpointLimit(sb)), true)
	if err != nil {
		return err
	}

	// Publish the loaded state and pin it.
	st.refs = new(refCount)
	db.cur.Store(st)
	db.flush.written(st.ckpt)
	return db.updatePin(context.Background())
}

// takeSlot locks the first free reader slot and returns its index.
func takeSlot(f *os.File) (int, error) {
	for i := range slots {
		ok, err := lock(f, lockSlot+int64(i), false)
		if err != nil {
			return 0, err
		}
		if ok {
			return i, nil
		}
	}
	return 0, ErrSlotsFull
}

// readSuper returns the newer valid superblock.
func (db *DB) readSuper() (superblock, error) {
	// Read both superblocks.
	b := make([]byte, 2*pageSize)
	if _, err := db.f.ReadAt(b, superPage*pageSize); err != nil {
		return superblock{}, err
	}

	// Pick the valid one with the higher generation.
	a, aok := decodeSuperblock(b)
	c, cok := decodeSuperblock(b[pageSize:])
	switch {
	case aok && (!cok || a.gen > c.gen):
		return a, nil
	case cok:
		return c, nil
	}
	return superblock{}, errors.Wrap(ErrCorrupt, "no valid superblock")
}

// tail returns st advanced to the newest checkpoint and the end of the
// valid log, or st itself when nothing changed. With verify set it checks
// the values of records the writer had not flushed, stopping before the
// first record whose values are torn.
func (db *DB) tail(st *state, verify bool) (*state, error) {
	// Adopt a newer checkpoint.
	sb, err := db.readSuper()
	if err != nil {
		return nil, err
	}
	next := st
	switch {
	case sb.gen <= st.gen:
	case sb.seq > st.seq:
		next = newState(sb, db.opts.checkpointLimit(sb))
	default:
		next = st.rebase(sb, db.opts.checkpointLimit(sb))
	}

	// Read the records after the state and where each one ends.
	type read struct {
		r        *record
		pos, end uint64
	}
	var recs []read
	var durable uint64
	lr := newLogReader(db.f, next.pos, next.end, next.seq)
	for {
		r, ok := lr.next()
		if !ok {
			break
		}
		recs = append(recs, read{r: r, pos: lr.pos, end: lr.end})
		durable = max(durable, r.durable)
	}
	if len(recs) == 0 {
		return next, nil
	}

	// Keep the records before the first unflushed one whose values are
	// missing, when asked to check.
	rs := make([]*record, 0, len(recs))
	for _, rd := range recs {
		if verify && rd.r.seq > durable && !db.verify(rd.r) {
			break
		}
		rs = append(rs, rd.r)
	}
	if len(rs) == 0 {
		return next, nil
	}

	// Apply them to a copy.
	if next == st {
		next = db.derive(st)
	}
	next.apply(rs...)
	last := recs[len(rs)-1]
	next.pos, next.end = last.pos, last.end
	return next, nil
}

// derive returns a copy of st whose overlay the caller owns. Copying a tree
// marks the source, so copies serialize under bcast.
func (db *DB) derive(st *state) *state {
	// Copy the state and its overlay; the copy holds no snapshots.
	next := *st
	next.refs = nil
	l := db.bcast.Lock()
	next.overlay = st.overlay.Copy()
	l.Unlock()
	return &next
}

// verify reports whether every value r references matches its checksum.
func (db *DB) verify(r *record) bool {
	for _, o := range r.ops {
		if o.val.isRef {
			if _, err := db.p.readValue(o.val); err != nil {
				return false
			}
		}
	}
	return true
}

// tailLoop applies records other processes append until the watcher stops.
// A failed tail leaves the published state; the next change retries it.
func (db *DB) tailLoop(ctx context.Context) error {
	for db.watch.wait() {
		if st, err := db.tail(db.cur.Load(), false); err == nil {
			_ = db.publish(ctx, st)
		}
	}
	return nil
}

// warm reads the tree into the cache within its budget.
func (db *DB) warm(ctx context.Context) error {
	st, stripe := db.acquire()
	defer db.release(st, stripe)
	return db.p.warm(ctx, st.root, db.opts.CacheBytes)
}

// publish makes st the published state unless the published state is as
// new, then raises the slot's pin when it can.
func (db *DB) publish(ctx context.Context, st *state) error {
	// Keep a published state at least as new. Two states of the same
	// commit and checkpoint hold the same data, so the first one stays.
	l := db.bcast.Lock()
	cur := db.cur.Load()
	if st == cur || st.seq < cur.seq || (st.seq == cur.seq && st.gen <= cur.gen) {
		l.Unlock()
		return nil
	}

	// Replace the state, keeping the old one until its snapshots end.
	st.refs = new(refCount)
	db.cur.Store(st)
	db.retired = append(db.retired, cur)
	l.Broadcast()
	l.Unlock()

	// Its checkpoint is in the file, whichever process wrote it.
	db.flush.written(st.ckpt)
	return db.updatePin(ctx)
}

// acquire returns the published state and counts it as an open snapshot in
// a random stripe, which the snapshot passes to release. A state stays
// readable while it is published or counted, so acquire retries when
// publish replaced the state before the count landed: the pin may already
// exclude it.
func (db *DB) acquire() (*state, int) {
	stripe := rand.IntN(refStripes) // #nosec G404 -- spreads contention; no secret.
	for {
		st := db.cur.Load()
		st.refs[stripe].n.Add(1)
		if db.cur.Load() == st {
			return st, stripe
		}
		st.refs[stripe].n.Add(-1)
	}
}

// release ends a snapshot of st counted in stripe. The pin moves only when
// a replaced state's last snapshot may have ended.
func (db *DB) release(st *state, stripe int) {
	if st.refs[stripe].n.Add(-1) != 0 || db.cur.Load() == st {
		return
	}
	_ = db.updatePin(context.Background())
}

// localPin returns the oldest state this handle's snapshots read, and
// forgets replaced states no snapshot reads.
func (db *DB) localPin() pin {
	// Start from the published state.
	l := db.bcast.Lock()
	defer l.Unlock()
	cur := db.cur.Load()
	p := pin{seq: cur.seq, ckpt: cur.ckpt}

	// Lower the pin by each replaced state still read.
	live := db.retired[:0]
	for _, s := range db.retired {
		if s.refs.held() {
			p = p.lower(pin{seq: s.seq, ckpt: s.ckpt})
			live = append(live, s)
		}
	}
	clear(db.retired[len(live):])
	db.retired = live
	return p
}

// updatePin writes the slot when the local pin moved. Snapshots only take
// the published state, so a pin computed under pinMtx never rises above a
// snapshot that starts later.
func (db *DB) updatePin(ctx context.Context) error {
	// Exclude other pin writers.
	release, err := db.pinMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Write the slot only when the pin moved.
	p := db.localPin()
	if p == db.pinned {
		return nil
	}
	return db.writePin(p)
}

// writePin writes p to this handle's slot. The caller holds pinMtx or owns
// the handle alone.
func (db *DB) writePin(p pin) error {
	db.pinned = p
	_, err := db.f.WriteAt(p.encode(), int64(slotPage*pageSize+db.slot*slotSize))
	return err
}

// minPin returns the oldest state any process's snapshots read.
func (db *DB) minPin() (pin, error) {
	// Read the slots.
	p := db.localPin()
	b := make([]byte, pageSize)
	if _, err := db.f.ReadAt(b, slotPage*pageSize); err != nil {
		return pin{}, err
	}

	// Lower the local pin by every other slot whose process holds its lock.
	for i := range slots {
		if i == db.slot {
			continue
		}
		sp, ok := decodePin(b[i*slotSize:])
		if !ok {
			continue
		}
		live, err := held(db.f, lockSlot+int64(i))
		if err != nil {
			return pin{}, err
		}
		if live {
			p = p.lower(sp)
		}
	}
	return p, nil
}

// lookup returns the value of key in st.
func (db *DB) lookup(st *state, key []byte) (value, bool, error) {
	if st.filter.has(key) {
		if it, ok := st.overlay.Get(overlayProbe(key)); ok {
			return it.e.val, !it.e.del, nil
		}
	}
	return db.p.get(st.root, key)
}

// NewTransaction opens a transaction. A write transaction holds the writer
// lock across all processes until Commit or Discard.
func (db *DB) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		st, stripe := db.acquire()
		return &Tx{db: db, st: st, stripe: stripe}, nil
	}
	if err := db.w.lock(ctx); err != nil {
		return nil, err
	}
	st, stripe := db.acquire()
	return &Tx{db: db, st: st, stripe: stripe, changes: newChanges()}, nil
}

// Seq returns the last commit of the published state.
func (db *DB) Seq() uint64 {
	return db.cur.Load().seq
}

// WaitSeq returns once the published state includes commit seq, from this
// process or another.
func (db *DB) WaitSeq(ctx context.Context, seq uint64) error {
	return db.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if db.closed {
			return false, ErrClosed
		}
		return db.cur.Load().seq >= seq, nil
	})
}

// Compact moves every value out of sparse pages and the file's tail,
// checkpoints, and releases freed space.
func (db *DB) Compact(ctx context.Context) error {
	if err := db.w.lock(ctx); err != nil {
		return err
	}
	defer db.w.unlock()
	return db.w.compact(ctx)
}

// Sync makes every earlier commit durable.
func (db *DB) Sync(ctx context.Context) error {
	return db.flush.syncThrough(ctx, db.cur.Load().seq, 0)
}

// WaitDurable returns once every earlier commit is durable without forcing
// a flush. An ordered commit is flushed within a second of completing.
func (db *DB) WaitDurable(ctx context.Context) error {
	return db.flush.waitDurable(ctx, db.cur.Load().seq)
}

// Close checkpoints when this handle can take the writer lock without
// waiting, flushes, and closes the file. Transactions must end first.
func (db *DB) Close() error {
	// Stop the warm-up so its snapshot does not hold space.
	if wait, _ := db.warmer.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Checkpoint and release when no other process is writing.
	ctx := context.Background()
	var err error
	if ok, lerr := db.w.tryLock(); lerr == nil && ok {
		if err = db.w.catchUp(ctx); err == nil {
			err = db.w.checkpointDurable(ctx)
		}
		if err == nil {
			err = db.w.releaseNow(ctx)
		}
		db.w.unlock()
	}

	// Stop tailing and the deadline flush, clear the reader slot, and
	// flush.
	db.flush.close()
	db.watch.stop()
	if wait, _ := db.tailer.SetRoutine(nil); wait != nil {
		<-wait
	}
	if _, werr := db.f.WriteAt(make([]byte, slotSize), int64(slotPage*pageSize+db.slot*slotSize)); err == nil {
		err = werr
	}
	if serr := flushDurable(db.f); err == nil {
		err = serr
	}

	// Close the watcher last: on darwin closing any descriptor of the file
	// drops the process's record locks.
	db.watch.close()
	if cerr := db.f.Close(); err == nil {
		err = cerr
	}
	openFiles.release(db.id)

	// Wake waiters.
	l := db.bcast.Lock()
	db.closed = true
	l.Broadcast()
	l.Unlock()
	return err
}

// _ is a type assertion
var (
	_ kvtx.Store              = (*DB)(nil)
	_ kvtx.OrderedCommitStore = (*DB)(nil)
)
