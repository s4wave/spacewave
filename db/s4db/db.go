// Package s4db is a single-file embedded key-value engine.
//
// Every commit appends its values over inlineMax to an open value chunk and
// one record to a log inside the file, and shares one durable flush with the
// commits beside it. Each process keeps the records since the last flush in a
// memtable; a background flush writes the memtable as one sorted run and reads
// nothing; background merges keep a key in a handful of runs; a cleaner
// evacuates value chunks whose live fraction fell. The file is whole 1 MiB
// chunks, allocated lowest first, so it shrinks by truncation and nothing is
// punched.
//
// Any number of processes may open the file. Each holds a reader slot that
// publishes the oldest state its snapshots read, and tails the log when the
// file changes. A write transaction holds the writer lock, so commits from
// all processes form one sequence, and every background step appends its
// record under the same lock.
package s4db

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

const (
	// MaxKeySize is the longest key.
	MaxKeySize = 1024
	// MaxValueSize is the longest value, the most a reference's length holds.
	MaxValueSize = math.MaxUint32
)

// DB is an open database file.
type DB struct {
	// s is the database file.
	s storage
	// hdr is the file header.
	hdr *header
	// opts holds the options with defaults applied.
	opts Options
	// id is this handle's writer identity, which its records carry.
	id uint64
	// slot is this handle's reader slot.
	slot int
	// watch wakes the tail loop when the file changes.
	watch watcher
	// flush shares durable flushes between commits.
	flush *flusher
	// w writes commits and background records.
	w *writer
	// tailer runs the tail loop.
	tailer *routine.RoutineContainer
	// cache holds run blocks, index partitions and filters.
	cache *blockCache
	// runs holds the readers of the runs this handle has read.
	runs runs
	// budget is the file's memtable budget, from its header.
	budget int64
	// super is the newest superblock this handle read at open or made
	// durable; the release step trusts its seq. superMtx serializes it.
	super    superblock
	superMtx sync.Mutex
	// sched runs the background jobs.
	sched *scheduler

	// cur is the published state. Read transactions acquire it without a
	// lock.
	cur atomic.Pointer[state]
	// closed is set once Close finishes. Close sets it under bcast, so a
	// waiter woken by the close sees it.
	closed atomic.Bool

	// bcast guards the fields below, serializes state publication, and wakes
	// waiters when a state is published or the database closes. It is held
	// only for memory work.
	bcast broadcast.Broadcast
	// retired holds earlier states that snapshots may still read.
	retired []*state
	// leases holds the lock bytes of this handle's leases and lease waits.
	leases map[int64]struct{}
	// writers counts this handle's write transactions from NewTransaction
	// until Discard, or until Commit returns with its changes durable.
	writers int
	// stopping is set when Close begins, refusing new write transactions.
	stopping bool
	// lockWaits counts lease lock calls in progress. The file stays open
	// until they return, so the system never grants a lock on a closed file.
	lockWaits sync.WaitGroup

	// pinMtx guards pinned and serializes writes to the slot.
	pinMtx csync.Mutex
	// pinned is the pin last written to the slot.
	pinned pin
	// ownSeq is the newest record this handle wrote. The writer stores it
	// before the write reaches the file, so a tailed state beyond it came
	// from another process.
	ownSeq atomic.Uint64
	// shared is set once a tail finds another process's record. Until then
	// the slot keeps the pin written at open, which is no newer than any
	// state this handle reads, so the slot is not rewritten on every commit.
	shared atomic.Bool
	// flows counts the values this handle's commits wrote and freed outside
	// the index, for each of Options.Families and then for the other keys.
	flows []flowCount
	// written counts the bytes this handle wrote to the file, and userBytes
	// the key and value bytes its commits carried.
	written, userBytes atomic.Int64
	// waitMax is the longest a commit of this handle waited for the writer
	// lock while a background step held it, in nanoseconds.
	waitMax atomic.Int64
}

// open opens the database in s, closing s on failure.
func open(s storage, opts Options) (*DB, error) {
	// Build the database on s with its flusher and writer.
	opts = opts.withDefaults()
	db := &DB{
		s:      s,
		opts:   opts,
		id:     rand.Uint64(), // #nosec G404 -- an identity, not a secret.
		tailer: routine.NewRoutineContainer(),
		leases: make(map[int64]struct{}),
		flows:  make([]flowCount, len(opts.Families)+1),
		cache:  newBlockCache(opts.blockCacheBytes()),
		runs:   runs{readers: make(map[uint64]*runReader)},
	}
	db.flush = newFlusher(s, &db.cur)
	db.w = &writer{db: db}

	// Recover the state, releasing everything on failure.
	if err := db.load(); err != nil {
		if db.watch != nil {
			db.watch.close()
		}
		_ = s.close()
		return nil, err
	}

	// Follow other processes' records, and run the background jobs.
	db.tailer.SetRoutine(db.tailLoop)
	db.tailer.SetContext(context.Background(), false)
	if !opts.ReadOnly {
		db.sched = newScheduler(db)
	}
	return db, nil
}

// load creates the file if empty, takes a reader slot, and recovers state.
func (db *DB) load() error {
	// Create the file under the writer lock so concurrent openers agree.
	if !db.opts.ReadOnly {
		if _, err := db.s.lock(lockWriter, true); err != nil {
			return err
		}
		err := db.w.create()
		if uerr := db.s.unlock(lockWriter); err == nil {
			err = uerr
		}
		if err != nil {
			return err
		}
	}

	// Read the header.
	page := make([]byte, pageSize)
	if _, err := db.s.ReadAt(page, headerPage*pageSize); err != nil {
		return err
	}
	var err error
	if db.hdr, err = decodeHeader(page); err != nil {
		return err
	}
	db.budget = int64(db.hdr.memtable) // #nosec G115 -- the header bounds it.

	// Take a free slot, pin everything until the state is loaded, and
	// watch the file.
	if db.slot, err = takeSlot(db.s); err != nil {
		return err
	}
	if err := db.writePin(pin{id: db.id}); err != nil {
		return err
	}
	if db.watch, err = db.s.watch(db.slot); err != nil {
		return err
	}

	// Load the superblock's manifest and the log after it, verifying the
	// values the writer had not flushed.
	sb, m, err := db.readSuper()
	if err != nil {
		return err
	}
	db.super = sb
	st, torn, err := db.tail(newState(sb, m, db.budget), true)
	if err != nil {
		return err
	}
	if torn && !db.opts.ReadOnly {
		if st, err = db.cutTorn(st); err != nil {
			return err
		}
	}

	// Publish the loaded state and pin it.
	st.refs = new(refCount)
	db.cur.Store(st)
	db.ownSeq.Store(st.seq)
	return db.writePin(db.localPin())
}

// takeSlot locks the first free reader slot and returns its index.
func takeSlot(s storage) (int, error) {
	for i := range slots {
		ok, err := s.lock(lockSlot+int64(i), false)
		if err != nil {
			return 0, err
		}
		if ok {
			return i, nil
		}
	}
	return 0, ErrSlotsFull
}

// Shared reports whether another process has the file open.
func (db *DB) Shared() (bool, error) {
	for i := range slots {
		if i == db.slot {
			continue
		}
		held, err := db.s.held(lockSlot + int64(i))
		if err != nil || held {
			return held, err
		}
	}
	return false, nil
}

// readSupers returns the valid superblocks the file holds, newer first.
func (db *DB) readSupers() ([]superblock, error) {
	// Read both pages and keep the valid ones.
	b := make([]byte, 2*pageSize)
	if _, err := db.s.ReadAt(b, superPage*pageSize); err != nil {
		return nil, err
	}
	var order []superblock
	for _, page := range [][]byte{b[:pageSize], b[pageSize:]} {
		if sb, ok := decodeSuperblock(page); ok {
			order = append(order, sb)
		}
	}
	if len(order) == 2 && order[1].gen > order[0].gen {
		order[0], order[1] = order[1], order[0]
	}
	return order, nil
}

// readSuper returns the newer valid superblock whose manifest reads, with
// that manifest. A newer superblock whose manifest is torn, because a crash
// came between the two writes, yields to the older one.
func (db *DB) readSuper() (superblock, *manifest, error) {
	// Take the newest whose manifest reads.
	order, err := db.readSupers()
	if err != nil {
		return superblock{}, nil, err
	}
	for _, sb := range order {
		var m *manifest
		if m, err = db.readManifest(sb); err == nil {
			return sb, m, nil
		}
	}
	if err == nil {
		err = errors.Wrap(ErrCorrupt, "no valid superblock")
	}
	return superblock{}, nil, err
}

// readManifest reads the manifest sb points at.
func (db *DB) readManifest(sb superblock) (*manifest, error) {
	b := make([]byte, sb.manifest.n)
	if _, err := db.s.ReadAt(b, fileOff(sb.manifest.off)); err != nil {
		return nil, err
	}
	if checksum(b) != sb.manifest.crc {
		return nil, errors.Wrap(ErrCorrupt, "manifest checksum mismatch")
	}
	return decodeManifest(b)
}

// writeSuper writes sb as the file's next superblock generation and flushes
// it, under lockSuper so processes publishing at once keep the generations
// ordered. The generation comes from the file, since another process may have
// published since this one read it; when the file already holds a superblock
// at or past sb.seq, sb is older than it and is not written. db.super keeps
// the newest superblock this handle made durable.
func (db *DB) writeSuper(sb superblock) error {
	// Serialize with the other publishers and this handle's readers of
	// db.super.
	if _, err := db.s.lock(lockSuper, true); err != nil {
		return err
	}
	defer func() { _ = db.s.unlock(lockSuper) }()
	db.superMtx.Lock()
	defer db.superMtx.Unlock()

	// Follow the newest generation in the file, unless it is already past sb.
	order, err := db.readSupers()
	if err != nil {
		return err
	}
	sb.gen = 1
	if len(order) != 0 {
		if order[0].seq >= sb.seq {
			return nil
		}
		sb.gen = order[0].gen + 1
	}

	// Write and flush it.
	if _, err := db.s.WriteAt(sb.encode(), sb.page()*pageSize); err != nil {
		return err
	}
	if err := db.s.flushDurable(); err != nil {
		return err
	}
	db.super = sb
	return nil
}

// tail returns st advanced to the end of the valid log, or st itself when
// nothing changed. With verify set it checks the values of records the writer
// had not flushed, stopping before the first record whose values are torn
// and reporting it.
func (db *DB) tail(st *state, verify bool) (*state, bool, error) {
	// Read the records after the state.
	var recs []*record
	var durable uint64
	lr := newLogReader(db.s, st.pos, st.end, st.seq, st.crc, st.writer)
	for {
		r, ok := lr.next()
		if !ok {
			break
		}
		r.own()
		recs = append(recs, r)
		durable = max(durable, r.durable)
	}
	if len(recs) == 0 {
		return st, false, nil
	}

	// Keep the records before the first unflushed one whose values are
	// missing, when asked to check.
	kept := recs
	torn := false
	for i, r := range recs {
		if verify && r.kind == kindCommit && r.seq > max(durable, st.logged) && !db.verify(r) {
			kept, torn = recs[:i], true
			break
		}
	}
	if len(kept) == 0 {
		return st, torn, nil
	}

	// Apply them to a copy.
	next := st.derive()
	for _, r := range kept {
		next.apply(r, db.budget)
	}
	return next, torn, nil
}

// cutTorn erases the record after st whose values a crash tore, so tails,
// which trust records, end the log where recovery did. Another process may
// have appended meanwhile, so it tails again under the writer lock first.
func (db *DB) cutTorn(st *state) (*state, error) {
	// Exclude writers and find the torn record again.
	if _, err := db.s.lock(lockWriter, true); err != nil {
		return nil, err
	}
	st, torn, err := db.tail(st, true)

	// Overwrite its header and flush.
	if err == nil && torn {
		if _, err = db.s.WriteAt(make([]byte, recordHeader), fileOff(st.pos)); err == nil {
			err = db.s.flushDurable()
		}
	}
	if uerr := db.s.unlock(lockWriter); err == nil {
		err = uerr
	}
	return st, err
}

// verify reports whether every value r references matches its checksum.
func (db *DB) verify(r *record) bool {
	for _, o := range r.ops {
		if o.val.isRef() {
			if _, err := db.readValue(o.val); err != nil {
				return false
			}
		}
	}
	return true
}

// tailLoop applies records other processes append until the watcher stops.
// It skips the wakes this handle's writer raises: while the writer lock is
// held no other process writes, and taking the lock applied what they wrote
// before. A failed tail leaves the published state; the next change retries
// it.
func (db *DB) tailLoop(ctx context.Context) error {
	for db.watch.wait() {
		if !db.w.held.Load() {
			_ = db.refresh(ctx)
		}
	}
	return nil
}

// refresh publishes the records other processes wrote, without waiting for
// the tail loop to notice them.
func (db *DB) refresh(ctx context.Context) error {
	st, _, err := db.tail(db.cur.Load(), false)
	if err != nil {
		return err
	}
	if st.seq > db.ownSeq.Load() {
		db.shared.Store(true)
	}
	return db.publish(ctx, st)
}

// publish makes st the published state unless the published state is as
// new, then raises the slot's pin when it can and another process reads it.
func (db *DB) publish(ctx context.Context, st *state) error {
	// Keep a published state at least as new.
	l := db.bcast.Lock()
	cur := db.cur.Load()
	if st == cur || st.seq <= cur.seq {
		l.Unlock()
		return nil
	}

	// Replace the state, keeping the old one until its snapshots end.
	st.refs = new(refCount)
	db.cur.Store(st)
	db.retired = append(db.retired, cur)
	l.Broadcast()
	l.Unlock()
	if !db.shared.Load() {
		return nil
	}
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
// a replaced state's last snapshot may have ended and another process reads
// the slot.
func (db *DB) release(st *state, stripe int) {
	if st.refs[stripe].n.Add(-1) != 0 || db.cur.Load() == st || !db.shared.Load() {
		return
	}
	_ = db.updatePin(context.Background())
}

// pin returns the pin a snapshot of st holds, for the handle id.
func (st *state) pin(id uint64) pin {
	return pin{seq: st.seq, version: st.m.version, id: id}
}

// localPin returns the oldest state this handle's snapshots read, and
// forgets replaced states no snapshot reads.
func (db *DB) localPin() pin {
	// Start from the published state.
	l := db.bcast.Lock()
	defer l.Unlock()
	p := db.cur.Load().pin(db.id)

	// Lower the pin by each replaced state still read.
	live := db.retired[:0]
	for _, s := range db.retired {
		if s.refs.held() {
			p = p.lower(s.pin(db.id))
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
	_, err := db.s.WriteAt(p.encode(), int64(slotPage*pageSize+db.slot*slotSize))
	return err
}

// lookup returns the value of key in st: the memtable decides when it holds
// the key, else the newest run that holds it.
func (db *DB) lookup(st *state, key []byte) (value, bool, error) {
	if e, ok := st.mem.get(key); ok {
		return e.val, e.val.kind != valueDeleted, nil
	}
	return db.lookupRuns(st, key)
}

// readValue returns the bytes of v: an inline value's own bytes, or the
// checked bytes a reference points at, in a new buffer.
func (db *DB) readValue(v value) ([]byte, error) {
	// An inline value is in memory.
	if !v.isRef() {
		return v.inline, nil
	}

	// Read and check the referenced bytes.
	b := make([]byte, v.ref.n)
	if _, err := db.s.ReadAt(b, fileOff(v.ref.off)); err != nil {
		return nil, err
	}
	if checksum(b) != v.ref.crc {
		return nil, errors.Wrapf(ErrCorrupt, "value checksum mismatch at %d", v.ref.off)
	}
	return b, nil
}

// NewTransaction opens a transaction. A write transaction holds the writer
// lock across all processes until Commit or Discard.
func (db *DB) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Refuse a closed handle and serve a read from the published state.
	if db.closed.Load() {
		return nil, ErrClosed
	}
	if !write {
		st, stripe := db.acquire()
		return &Tx{db: db, st: st, stripe: stripe}, nil
	}
	if db.opts.ReadOnly {
		return nil, ErrReadOnly
	}

	// Wait while the memtable is past its ceiling, then count the writer so
	// Close waits for it, take the writer lock and read the state it caught
	// up to.
	if err := db.waitMemtable(ctx, block.IsBackground(ctx)); err != nil {
		return nil, err
	}
	if err := db.beginWrite(); err != nil {
		return nil, err
	}
	if err := db.w.lock(ctx); err != nil {
		db.endWrite()
		return nil, err
	}
	st, stripe := db.acquire()
	return &Tx{db: db, st: st, stripe: stripe, changes: newChanges()}, nil
}

// beginWrite counts a write transaction, refusing it once Close began.
func (db *DB) beginWrite() error {
	// Count the writer unless Close began.
	l := db.bcast.Lock()
	defer l.Unlock()
	if db.stopping {
		return ErrClosed
	}
	db.writers++
	return nil
}

// endWrite ends a write transaction beginWrite counted.
func (db *DB) endWrite() {
	l := db.bcast.Lock()
	db.writers--
	l.Broadcast()
	l.Unlock()
}

// drainWriters refuses new write transactions and waits up to
// closeDrainTimeout for the counted ones to end, so a commit's writes and
// durable flush finish before Close flushes and closes the file. A writer
// still open after the wait is left to fail on the closed file.
func (db *DB) drainWriters() {
	// Refuse new writers.
	l := db.bcast.Lock()
	db.stopping = true
	l.Broadcast()
	l.Unlock()

	// Wait for the counted ones.
	ctx, cancel := context.WithTimeout(context.Background(), closeDrainTimeout)
	defer cancel()
	_ = db.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return db.writers == 0, nil
	})
}

// closeDrainTimeout bounds how long Close waits for write transactions.
const closeDrainTimeout = 10 * time.Second

// WritersWaiting reports whether a writer of this handle waits for the writer
// lock, so a holder doing deferrable work can commit early and let it in.
func (db *DB) WritersWaiting() bool {
	return db.w.waiting.Load() != 0
}

// SupportsAtomicCommit reports true: Commit applies a write transaction in
// full and durably before it returns, and Discard drops it.
func (db *DB) SupportsAtomicCommit() bool {
	return true
}

// Seq returns the last record of the published state.
func (db *DB) Seq() uint64 {
	return db.cur.Load().seq
}

// WaitSeq returns once the published state includes record seq, from this
// process or another.
func (db *DB) WaitSeq(ctx context.Context, seq uint64) error {
	return db.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if db.closed.Load() {
			return false, ErrClosed
		}
		return db.cur.Load().seq >= seq, nil
	})
}

// Sync makes every earlier commit durable.
func (db *DB) Sync(ctx context.Context) error {
	return db.flush.syncThrough(ctx, db.cur.Load().seq)
}

// WaitDurable returns once every earlier commit is durable without forcing
// a flush. An ordered commit is flushed within a second of completing.
func (db *DB) WaitDurable(ctx context.Context) error {
	return db.flush.waitDurable(ctx, db.cur.Load().seq)
}

// Close flushes and closes the file. It leaves the memtable in the log for
// the next open to replay, which is bounded by the flush budget.
// Transactions should end first; Close refuses new write transactions and
// waits a bounded time for open ones to commit or discard. A lease still held
// ends with the file, which drops its lock, and releasing it afterwards
// returns nil. Close waits for an abandoned lease wait to return.
func (db *DB) Close() error {
	// Let this handle's write transactions finish their commits, then stop
	// the background jobs and append the closing mark.
	db.drainWriters()
	if db.sched != nil {
		db.sched.stop()
		db.closingMark()
	}

	// Stop the deadline flush and tailing, clear the reader slot, and flush.
	db.flush.close()
	db.watch.stop()
	if wait, _ := db.tailer.SetRoutine(nil); wait != nil {
		<-wait
	}
	var err error
	if !db.opts.ReadOnly {
		if _, werr := db.s.WriteAt(make([]byte, slotSize), int64(slotPage*pageSize+db.slot*slotSize)); err == nil {
			err = werr
		}
		if serr := db.s.flushDurable(); err == nil {
			err = serr
		}
	}

	// Mark the handle closed and wake waiters before the file goes away, so a
	// lease released from here on leaves the descriptor alone.
	l := db.bcast.Lock()
	db.closed.Store(true)
	l.Broadcast()
	l.Unlock()

	// Close the watcher last, once no lease lock call is pending: on darwin
	// closing any descriptor of the file drops the process's record locks.
	db.lockWaits.Wait()
	db.watch.close()
	if cerr := db.s.close(); err == nil {
		err = cerr
	}
	return err
}

// closingMark flushes and appends a mark record when this handle can take the
// writer lock without waiting, so the next open verifies no values and the
// value chunks the last commits freed can be reused.
func (db *DB) closingMark() {
	// Take the lock only when it is free; another process's writer is
	// writing marks of its own.
	ctx := context.Background()
	ok, err := db.w.tryLock()
	if err != nil || !ok {
		return
	}
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err == nil {
		_, _ = db.w.appendRecord(ctx, &record{kind: kindCommit})
	}
	db.w.unlock()
}

// _ is a type assertion
var (
	_ kvtx.Store              = (*DB)(nil)
	_ kvtx.OrderedCommitStore = (*DB)(nil)
	_ kvtx.AtomicCommitStore  = (*DB)(nil)
	_ kvtx.WriterQueueStore   = (*DB)(nil)
)
