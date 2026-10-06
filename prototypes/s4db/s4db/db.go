//go:build darwin || linux

// Package s4db is a single-file embedded key-value engine.
//
// Every commit appends one record to a log inside the file. Each process
// keeps the records since the last checkpoint in an in-memory overlay above a
// copy-on-write B+tree, which a checkpoint rewrites once for many commits.
// Values above a size threshold live outside the tree, packed into extents
// whose pages are punched out of the file as soon as no snapshot reads them.
//
// Any number of processes may open the file. Each holds a reader slot that
// publishes the oldest state its snapshots read, and tails the log when the
// file changes. A write transaction holds the writer lock, so commits from
// all processes form one sequence.
package s4db

import (
	"bytes"
	"context"
	"math"
	"math/rand/v2"
	"os"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// MaxKeySize is the longest key.
const MaxKeySize = 1024

// Options configures a database.
type Options struct {
	// InlineMax is the largest value stored in the index when creating a
	// file. Zero selects 512 bytes.
	InlineMax int
	// CachePages bounds the decoded index pages kept in memory. Zero
	// selects 16384 pages.
	CachePages int
	// CheckpointMin and CheckpointMax bound the overlay size that starts a
	// checkpoint; between them a checkpoint starts at a quarter of the tree.
	// Zero selects 4 MiB and 64 MiB.
	CheckpointMin, CheckpointMax int64
	// RelocateBudget is the value bytes compaction may move after each
	// commit. Zero selects 4 MiB; negative disables compaction.
	RelocateBudget int64
}

// relocateVisits bounds the entries one incremental relocation pass reads, so
// a commit's compaction work does not grow with the key count.
const relocateVisits = 4096

// compactBatch bounds the value bytes Compact holds in memory and commits
// at once.
const compactBatch = 64 << 20

// DB is an open database file.
type DB struct {
	// f is the database file.
	f *os.File
	// hdr is the file header.
	hdr *header
	// opts holds the options with defaults applied.
	opts Options
	// cache holds decoded index pages.
	cache *cache
	// slot is this handle's reader slot.
	slot int
	// watch wakes the tail loop when the file changes.
	watch *watcher
	// tailDone is closed when the tail loop exits.
	tailDone chan struct{}
	// closing is closed when Close starts.
	closing chan struct{}
	// warmDone is closed when the cache warm-up exits.
	warmDone chan struct{}

	// cur is the published state. Read transactions acquire it without
	// taking mtx.
	cur atomic.Pointer[state]

	// mtx guards the fields below.
	mtx sync.Mutex
	// retired holds earlier states that snapshots may still read.
	retired []*state
	// pinned is the pin last written to the slot.
	pinned pin
	// durable is the last commit known flushed to the drive.
	durable uint64
	// flushing is set while one caller flushes for every waiting commit.
	flushing bool
	// flushed is closed when the running flush ends.
	flushed chan struct{}

	// wmtx is held by the write transaction of this handle. The fields below
	// belong to the writer.
	wmtx sync.Mutex
	// sp is the space state, current through spSeq and spGen.
	sp *space
	// spSeq and spGen identify the state sp reflects.
	spSeq, spGen uint64
	// fileEnd is the file length in pages.
	fileEnd uint64
	// relocKey is where the next compaction pass resumes.
	relocKey []byte
}

// state is one immutable view of the database.
type state struct {
	// seq is the last record applied.
	seq uint64
	// ckpt is the record the tree includes through.
	ckpt uint64
	// gen is the superblock generation of the tree.
	gen uint64
	// sb is the superblock of the tree.
	sb superblock
	// root is the tree root page.
	root uint64
	// count is the number of keys.
	count int64
	// overlay holds changes after ckpt.
	overlay *btree.BTreeG[*oentry]
	// filter holds every key the overlay has held since ckpt, so lookups of
	// other keys skip the overlay.
	filter *filter
	// overlayBytes estimates the overlay's encoded size.
	overlayBytes int64
	// pos is the offset of the next record and end is the end of its chunk.
	pos, end uint64
	// refs counts the snapshots reading the state; publish sets it.
	refs *refCount
}

// refStripes is the number of counters a state's snapshot count spreads
// over, so concurrent readers do not share one cache line.
const refStripes = 16

// refCount counts a state's snapshots in stripes. A state is read while any
// stripe is nonzero.
type refCount [refStripes]struct {
	// n is this stripe's count.
	n atomic.Int64
	// _ pads the stripe to its own cache line.
	_ [56]byte
}

// held reports whether any snapshot reads the state.
func (r *refCount) held() bool {
	for i := range r {
		if r[i].n.Load() != 0 {
			return true
		}
	}
	return false
}

// oentry is one change in the overlay.
type oentry struct {
	// key is the key.
	key []byte
	// del marks a deleted key.
	del bool
	// val is the stored value.
	val value
	// seq is the commit that made the change.
	seq uint64
}

// lessOEntry orders overlay entries by key.
func lessOEntry(a, b *oentry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// newOverlay returns an empty overlay.
func newOverlay() *btree.BTreeG[*oentry] {
	return btree.NewBTreeGOptions(lessOEntry, btree.Options{NoLocks: true})
}

// Open opens or creates the database at path.
func Open(path string, opts Options) (*DB, error) {
	// Apply defaults.
	if opts.InlineMax == 0 {
		opts.InlineMax = 512
	}
	if opts.CachePages == 0 {
		opts.CachePages = 16384
	}
	if opts.CheckpointMin == 0 {
		opts.CheckpointMin = 4 << 20
	}
	if opts.CheckpointMax == 0 {
		opts.CheckpointMax = 64 << 20
	}
	if opts.RelocateBudget == 0 {
		opts.RelocateBudget = 4 << 20
	}

	// Watch the file before taking locks: on darwin, closing the watcher's
	// descriptor after a failure would drop them.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	db := &DB{
		f:        f,
		opts:     opts,
		cache:    newCache(opts.CachePages),
		tailDone: make(chan struct{}),
		closing:  make(chan struct{}),
		warmDone: make(chan struct{}),
	}
	if db.watch, err = newWatcher(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := db.open(); err != nil {
		db.watch.close()
		_ = f.Close()
		return nil, err
	}
	go db.tailLoop()
	go db.warm()
	return db, nil
}

// open creates the file if empty, takes a reader slot, and recovers state.
func (db *DB) open() error {
	// Create the file under the writer lock so concurrent openers agree.
	if _, err := lock(db.f, lockWriter, true); err != nil {
		return err
	}
	err := db.create()
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

	// Take a free slot and pin everything until the state is loaded.
	db.slot = -1
	for i := range slots {
		ok, err := lock(db.f, lockSlot+int64(i), false)
		if err != nil {
			return err
		}
		if ok {
			db.slot = i
			break
		}
	}
	if db.slot < 0 {
		return errors.New("every reader slot is in use")
	}
	if err := db.writePin(pin{}); err != nil {
		return err
	}

	// Load the checkpoint and the log after it, verifying values the writer
	// had not flushed.
	sb, err := db.readSuper()
	if err != nil {
		return err
	}
	st, err := db.tail(db.stateOf(sb), true)
	if err != nil {
		return err
	}

	// Publish the loaded state and pin it.
	db.mtx.Lock()
	defer db.mtx.Unlock()
	st.refs = new(refCount)
	db.cur.Store(st)
	return db.updatePin()
}

// create writes an empty database into an empty file.
func (db *DB) create() error {
	// Leave an existing file alone.
	info, err := db.f.Stat()
	if err != nil || info.Size() != 0 {
		return err
	}

	// Write the header and an empty slot table.
	hdr := &header{version: 1, checksum: checksumCRC32C, index: indexLogTree, values: valuesPacked, inlineMax: uint32(db.opts.InlineMax)}
	if _, err := db.f.WriteAt(hdr.encode(), headerPage*pageSize); err != nil {
		return err
	}
	if _, err := db.f.WriteAt(make([]byte, pageSize), slotPage*pageSize); err != nil {
		return err
	}

	// Allocate the first log chunk and save the space.
	sp := newSpace()
	chunk := sp.alloc(extentPages)
	sp.chunks = []run{chunk}
	ref, err := db.writeSpace(sp)
	if err != nil {
		return err
	}

	// Point the first superblock at the empty log and flush.
	sb := superblock{gen: 1, space: ref, logPos: chunk.start * pageSize, logEnd: (chunk.start + chunk.n) * pageSize}
	if err := db.f.Truncate(int64(sp.end * pageSize)); err != nil {
		return err
	}
	if _, err := db.f.WriteAt(sb.encode(), superPage*pageSize); err != nil {
		return err
	}
	return flushDurable(db.f)
}

// stateOf returns the state of a checkpoint with no later records.
func (db *DB) stateOf(sb superblock) *state {
	return &state{
		seq: sb.seq, ckpt: sb.seq, gen: sb.gen, sb: sb, root: sb.root, count: int64(sb.count),
		overlay: newOverlay(), filter: newFilter(db.checkpointLimit(sb)), pos: sb.logPos, end: sb.logEnd,
	}
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
	return superblock{}, errors.New("no valid superblock")
}

// tail returns st advanced to the newest checkpoint and the end of the
// valid log. With verify set it checks the values of records the writer had
// not flushed, stopping before the first record whose values are torn.
func (db *DB) tail(st *state, verify bool) (*state, error) {
	// Adopt a newer checkpoint.
	sb, err := db.readSuper()
	if err != nil {
		return nil, err
	}
	next := *st
	switch {
	case sb.gen <= st.gen:
	case sb.seq > st.seq:
		next = *db.stateOf(sb)
	default:
		next.ckpt, next.gen, next.sb, next.root = sb.seq, sb.gen, sb, sb.root
		next.overlay = newOverlay()
		next.filter = newFilter(db.checkpointLimit(sb))
		next.overlayBytes = 0
		st.overlay.Scan(func(e *oentry) bool {
			if e.seq > sb.seq {
				next.overlay.Set(e)
				next.filter.add(e.key)
				next.overlayBytes += overlaySize(e)
			}
			return true
		})
	}

	// Read the records after the state.
	lr := &logReader{f: db.f, pos: next.pos, end: next.end, seq: next.seq + 1}
	var recs []*record
	var durable uint64
	for {
		r, ok := lr.next()
		if !ok {
			break
		}
		recs = append(recs, r)
		durable = max(durable, r.durable)
	}
	if len(recs) == 0 && next.gen == st.gen {
		return st, nil
	}

	// Apply them, checking unflushed values when asked.
	if next.overlay == st.overlay {
		next.overlay = st.overlay.Copy()
	}
	lr = &logReader{f: db.f, pos: next.pos, end: next.end, seq: next.seq + 1}
	for _, r := range recs {
		if verify && r.seq > durable && !db.verify(r) {
			break
		}
		lr.next()
		next.apply(r)
		next.pos, next.end = lr.pos, lr.end
	}
	return &next, nil
}

// verify reports whether every value r references matches its checksum.
func (db *DB) verify(r *record) bool {
	for _, o := range r.ops {
		if o.val.isRef {
			if _, err := db.readValue(o.val); err != nil {
				return false
			}
		}
	}
	return true
}

// apply applies a record to st, whose overlay the caller owns.
func (st *state) apply(r *record) {
	st.seq = r.seq
	if r.kind != kindCommit {
		return
	}
	st.count += r.delta
	for _, o := range r.ops {
		e := &oentry{key: o.key, del: o.del, val: o.val, seq: r.seq}
		st.filter.add(e.key)
		if old, ok := st.overlay.Set(e); ok {
			st.overlayBytes -= overlaySize(old)
		}
		st.overlayBytes += overlaySize(e)
	}
}

// overlaySize estimates the bytes an overlay entry adds to a checkpoint.
func overlaySize(e *oentry) int64 {
	return int64(leafEntrySize(e.key, e.val))
}

// tailLoop applies records other processes append until Close.
func (db *DB) tailLoop() {
	defer close(db.tailDone)
	for db.watch.wait() {
		db.mtx.Lock()
		if st, err := db.tail(db.cur.Load(), false); err == nil {
			db.publish(st)
		}
		db.mtx.Unlock()
	}
}

// acquire returns the published state and counts it as an open snapshot in
// a random stripe, which the snapshot passes to release. A state stays
// readable while it is published or counted, so acquire retries when
// publish replaced the state before the count landed: the pin may already
// exclude it.
func (db *DB) acquire() (*state, int) {
	stripe := rand.IntN(refStripes)
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
	db.mtx.Lock()
	defer db.mtx.Unlock()
	_ = db.updatePin()
}

// publish makes st the published state unless a newer one is published. The
// caller holds mtx.
func (db *DB) publish(st *state) {
	// Keep a newer published state.
	cur := db.cur.Load()
	if st == cur || st.seq < cur.seq || (st.seq == cur.seq && st.gen < cur.gen) {
		return
	}

	// Replace the state, keeping the old one until its snapshots end.
	st.refs = new(refCount)
	db.cur.Store(st)
	db.retired = append(db.retired, cur)
	_ = db.updatePin()
}

// localPin returns the oldest state this handle's snapshots read, and
// forgets replaced states no snapshot reads. The caller holds mtx.
func (db *DB) localPin() pin {
	// Start from the published state.
	cur := db.cur.Load()
	p := pin{seq: cur.seq, ckpt: cur.ckpt}

	// Lower the pin by each replaced state still read.
	live := db.retired[:0]
	for _, s := range db.retired {
		if s.refs.held() {
			p.seq, p.ckpt = min(p.seq, s.seq), min(p.ckpt, s.ckpt)
			live = append(live, s)
		}
	}
	clear(db.retired[len(live):])
	db.retired = live
	return p
}

// updatePin writes the slot when the local pin moved. The caller holds mtx.
func (db *DB) updatePin() error {
	p := db.localPin()
	if p == db.pinned {
		return nil
	}
	return db.writePin(p)
}

// writePin writes p to this handle's slot.
func (db *DB) writePin(p pin) error {
	db.pinned = p
	_, err := db.f.WriteAt(encodeSlot(p), int64(slotPage*pageSize+db.slot*slotSize))
	return err
}

// minPin returns the oldest state any process's snapshots read.
func (db *DB) minPin() (pin, error) {
	// Start from this handle's pin.
	db.mtx.Lock()
	p := db.localPin()
	db.mtx.Unlock()

	// Lower it by every other slot whose process still holds its lock.
	b := make([]byte, pageSize)
	if _, err := db.f.ReadAt(b, slotPage*pageSize); err != nil {
		return pin{}, err
	}
	for i := range slots {
		if i == db.slot {
			continue
		}
		sp, ok := decodeSlot(b[i*slotSize:])
		if !ok {
			continue
		}
		live, err := held(db.f, lockSlot+int64(i))
		if err != nil {
			return pin{}, err
		}
		if live {
			p.seq, p.ckpt = min(p.seq, sp.seq), min(p.ckpt, sp.ckpt)
		}
	}
	return p, nil
}

// readValue returns a stored value, checking packed bytes against their
// checksum.
func (db *DB) readValue(v value) ([]byte, error) {
	// An inline value is in the index.
	if !v.isRef {
		return v.inline, nil
	}

	// Read and check packed bytes.
	b := make([]byte, v.ref.n)
	if _, err := db.f.ReadAt(b, int64(v.ref.off)); err != nil {
		return nil, err
	}
	if checksum(b) != v.ref.crc {
		return nil, errors.Errorf("value checksum mismatch at %d", v.ref.off)
	}
	return b, nil
}

// node returns a decoded index page.
func (db *DB) node(page uint64) (*node, error) {
	// Serve a cached page.
	if n := db.cache.get(page); n != nil {
		return n, nil
	}

	// Read, decode, and cache it.
	b := make([]byte, pageSize)
	if _, err := db.f.ReadAt(b, int64(page*pageSize)); err != nil {
		return nil, err
	}
	n, err := decodeNode(b)
	if err != nil {
		return nil, errors.Wrapf(err, "page %d", page)
	}
	db.cache.put(page, n)
	return n, nil
}

// root returns the decoded root page, keeping the last one outside the
// shards.
func (db *DB) root(page uint64) (*node, error) {
	if r := db.cache.root.Load(); r != nil && r.page == page {
		return r.n, nil
	}
	n, err := db.node(page)
	if err == nil {
		db.cache.root.Store(&cacheSlot{page: page, n: n})
	}
	return n, err
}

// child returns child i of inner page n, linking decoded inner children
// into n so later lookups skip the cache.
func (db *DB) child(n *node, i int) (*node, error) {
	if c := n.inner[i].Load(); c != nil {
		return c, nil
	}
	c, err := db.node(n.kids[i])
	if err == nil && !c.leaf {
		n.inner[i].Store(c)
	}
	return c, err
}

// lookup returns the value of key in st.
func (db *DB) lookup(st *state, key []byte) (value, bool, error) {
	if st.filter.has(key) {
		if e, ok := st.overlay.Get(&oentry{key: key}); ok {
			return e.val, !e.del, nil
		}
	}
	return treeGet(db, st.root, key)
}

// NewTransaction opens a transaction. A write transaction holds the writer
// lock across all processes until Commit or Discard.
func (db *DB) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		st, stripe := db.acquire()
		return &Tx{db: db, st: st, stripe: stripe}, nil
	}
	if err := db.lockWriter(); err != nil {
		return nil, err
	}
	st, stripe := db.acquire()
	return &Tx{db: db, st: st, stripe: stripe, changes: newChanges()}, nil
}

// lockWriter takes the writer lock and brings the writer state up to date
// with records other processes appended.
func (db *DB) lockWriter() error {
	// Exclude this handle's writers, then other processes.
	db.wmtx.Lock()
	if _, err := lock(db.f, lockWriter, true); err != nil {
		db.wmtx.Unlock()
		return err
	}

	// Apply what other processes wrote.
	err := db.catchUp()
	if err != nil {
		db.unlockWriter()
	}
	return err
}

// unlockWriter releases the writer lock.
func (db *DB) unlockWriter() {
	_ = unlock(db.f, lockWriter)
	db.wmtx.Unlock()
}

// catchUp applies new records and rebuilds the space when another process
// wrote since this handle last did. The caller holds the writer lock.
func (db *DB) catchUp() error {
	// Apply the new records and keep the space when it is still current.
	db.mtx.Lock()
	st, err := db.tail(db.cur.Load(), false)
	if err == nil {
		db.publish(st)
		st = db.cur.Load()
	}
	db.mtx.Unlock()
	if err != nil {
		return err
	}
	if db.sp != nil && db.spSeq == st.seq && db.spGen == st.gen {
		return nil
	}

	// Load the checkpoint's space.
	b := make([]byte, st.sb.space.n)
	if _, err := db.f.ReadAt(b, int64(st.sb.space.off)); err != nil {
		return err
	}
	if checksum(b) != st.sb.space.crc {
		return errors.New("space checksum mismatch")
	}
	sp, err := decodeSpace(b)
	if err != nil {
		return err
	}

	// Replay the space changes of the records after it.
	lr := &logReader{f: db.f, pos: st.sb.logPos, end: st.sb.logEnd, seq: st.sb.seq + 1}
	for lr.seq <= st.seq {
		r, ok := lr.next()
		if !ok {
			return errors.Errorf("log ends before record %d", st.seq)
		}
		if r.kind == kindLink {
			sp.claim(r.next)
			sp.chunks = append(sp.chunks, r.next)
			continue
		}
		// Release first: the writer placed this record's values after
		// every release its marks cover.
		sp.release(r.released.seq, r.released.ckpt)
		for _, o := range r.ops {
			if o.val.isRef {
				sp.useValue(o.val.ref.off, int(o.val.ref.n))
			}
		}
		for _, f := range r.frees {
			sp.freeValue(f.off, int(f.n), r.seq)
		}
	}

	// Adopt the space; its writer already punched what it released.
	sp.drain()
	db.sp, db.spSeq, db.spGen = sp, st.seq, st.gen
	info, err := db.f.Stat()
	if err != nil {
		return err
	}
	db.fileEnd = uint64(info.Size()) / pageSize

	// Flush whatever the previous writer left unflushed.
	if err := flushDurable(db.f); err != nil {
		return err
	}
	db.setDurable(st.seq)
	return nil
}

// setDurable advances durable.
func (db *DB) setDurable(seq uint64) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	db.durable = max(db.durable, seq)
}

// commit appends a commit record for changes on base, writes its values,
// publishes the new state, and runs checkpoint and compaction work. The
// caller holds the writer lock.
func (db *DB) commit(base *state, changes []tentry, ordered bool) error {
	// Resolve each change against the base state: frees, count, and value
	// placement.
	sp := db.sp
	r := &record{kind: kindCommit}
	var frees []extentRef
	var writes []valueWrite
	for _, c := range changes {
		old, found, err := db.lookup(base, c.key)
		if err != nil {
			return err
		}
		if c.del && !found {
			continue
		}
		if found && old.isRef {
			frees = append(frees, old.ref)
		}
		switch {
		case c.del:
			r.delta--
			r.ops = append(r.ops, op{key: c.key, del: true})
			continue
		case !found:
			r.delta++
		}
		v := value{inline: c.data}
		if len(c.data) > int(db.hdr.inlineMax) {
			v = value{isRef: true, ref: extentRef{n: uint32(len(c.data)), crc: checksum(c.data)}}
			writes = append(writes, valueWrite{op: len(r.ops), data: c.data})
		}
		r.ops = append(r.ops, op{key: c.key, val: v})
	}
	if len(r.ops) == 0 {
		return nil
	}

	// Continue the log in a new chunk when the record may not fit.
	bound := uint64(recordHeader + 64 + 20*len(frees) + linkSize)
	for _, o := range r.ops {
		bound += uint64(10 + len(o.key) + o.val.size())
	}
	seq := base.seq + 1
	pos, end := base.pos, base.end
	var link []byte
	linkPos := pos
	if pos+bound > end {
		chunk := sp.alloc(max(extentPages, (bound+pageSize-1)/pageSize))
		sp.chunks = append(sp.chunks, chunk)
		link = encodeLink(seq, chunk)
		seq++
		pos, end = chunk.start*pageSize, (chunk.start+chunk.n)*pageSize
	}
	r.seq = seq

	// Place values, then free replaced ones and release what no snapshot
	// reads.
	for _, w := range writes {
		ref := &r.ops[w.op].val.ref
		ref.off = sp.placeValue(len(w.data), seq)
	}
	for _, f := range frees {
		sp.freeValue(f.off, int(f.n), seq)
		r.frees = append(r.frees, f)
	}
	if err := db.releaseSpace(base, r); err != nil {
		return err
	}

	// Write values in runs of adjacent placements.
	if err := db.writeValues(r, writes); err != nil {
		return err
	}

	// Write the record and publish the new state together, so the tail loop
	// never reads a record of this handle before it is published. Copying a
	// tree writes to it, so copies also happen under mtx.
	db.mtx.Lock()
	r.durable = db.durable
	rec := encodeCommit(r)
	err := db.writeRecord(link, linkPos, rec, pos)
	if err == nil {
		next := *base
		next.overlay = base.overlay.Copy()
		next.apply(r)
		next.pos, next.end = pos+uint64(len(rec)), end
		db.publish(&next)
	}
	db.mtx.Unlock()
	if err != nil {
		return err
	}

	// Order the next commit's writes after this one's and release space.
	// A durable commit flushes after the writer lock is released.
	if ordered {
		if err := flushOrdered(db.f); err != nil {
			return err
		}
	}
	if err := db.punch(); err != nil {
		return err
	}

	// The space now reflects the record.
	db.spSeq = seq
	return nil
}

// writeRecord writes a commit record at pos, after the link that leads to
// its chunk when link is set.
func (db *DB) writeRecord(link []byte, linkPos uint64, rec []byte, pos uint64) error {
	if link != nil {
		if _, err := db.f.WriteAt(link, int64(linkPos)); err != nil {
			return err
		}
	}
	_, err := db.f.WriteAt(rec, int64(pos))
	return err
}

// valueWrite is a large value awaiting its write.
type valueWrite struct {
	// op indexes the record op holding the value's reference.
	op int
	// data is the value.
	data []byte
}

// writeValues writes placed values, joining adjacent ones into one write.
func (db *DB) writeValues(r *record, writes []valueWrite) error {
	// Collect adjacent values in buf, starting at at.
	var buf []byte
	var at uint64
	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		_, err := db.f.WriteAt(buf, int64(at))
		buf = buf[:0]
		return err
	}

	// Write each run when the next value is not adjacent.
	for _, w := range writes {
		off := r.ops[w.op].val.ref.off
		if len(buf) != 0 && at+uint64(len(buf)) != off {
			if err := flush(); err != nil {
				return err
			}
		}
		if len(buf) == 0 {
			at = off
		}
		buf = append(buf, w.data...)
	}
	return flush()
}

// releaseSpace returns freed runs no snapshot reads to free space and
// records the space's release marks in r. The marks only rise, so replaying
// any record repeats every release the writer made before it.
func (db *DB) releaseSpace(base *state, r *record) error {
	// Skip reading the slots when nothing awaits release.
	sp := db.sp
	defer func() { r.released = sp.marks }()
	if len(sp.values) == 0 && len(sp.pages) == 0 {
		return nil
	}

	// Release value runs freed by durable commits no snapshot predates, and
	// pages freed by checkpoints no snapshot predates.
	p, err := db.minPin()
	if err != nil {
		return err
	}
	db.mtx.Lock()
	durable := db.durable
	db.mtx.Unlock()
	sp.release(min(p.seq, durable), min(p.ckpt, base.ckpt))
	return nil
}

// punch deallocates released runs and shrinks the file to the space end.
func (db *DB) punch() error {
	// Deallocate released runs below the end.
	for _, r := range db.sp.drain() {
		if r.start >= db.sp.end {
			continue
		}
		db.cache.drop(r.start, r.n)
		if err := punch(db.f, int64(r.start*pageSize), int64(min(r.n, db.sp.end-r.start)*pageSize)); err != nil {
			return err
		}
	}

	// Cut the file at the end.
	if db.sp.end < db.fileEnd {
		db.cache.drop(db.sp.end, db.fileEnd-db.sp.end)
		if err := db.f.Truncate(int64(db.sp.end * pageSize)); err != nil {
			return err
		}
	}
	db.fileEnd = db.sp.end
	return nil
}

// checkpointLimit returns the overlay size that starts a checkpoint after
// the checkpoint sb.
func (db *DB) checkpointLimit(sb superblock) int64 {
	return min(max(int64(sb.treePages)*pageSize/4, db.opts.CheckpointMin), db.opts.CheckpointMax)
}

// checkpointDue reports whether the overlay of st should be written into
// the tree.
func (db *DB) checkpointDue(st *state) bool {
	return st.overlayBytes > db.checkpointLimit(st.sb)
}

// checkpoint writes the overlay into the tree and saves a superblock. The
// caller holds the writer lock.
func (db *DB) checkpoint() error {
	// Skip when no commit follows the last checkpoint.
	st := db.cur.Load()
	if st.seq == st.ckpt {
		return nil
	}
	sp := db.sp

	// Rewrite the changed pages into one new run.
	changes := make([]change, 0, st.overlay.Len())
	st.overlay.Scan(func(e *oentry) bool {
		changes = append(changes, change{key: e.key, del: e.del, val: e.val})
		return true
	})
	b := &builder{p: db}
	top, err := b.build(st.root, changes)
	if err != nil {
		return err
	}
	var root uint64
	var fresh int
	if top != nil {
		root = top.page
		if fresh = b.reachable(top); fresh != 0 {
			r := sp.alloc(uint64(fresh))
			nums, nodes, buf := b.place(top, r.start)
			if _, err := db.f.WriteAt(buf, int64(r.start*pageSize)); err != nil {
				return err
			}
			for i, n := range nodes {
				db.cache.put(nums[i], n)
			}
			root = nums[len(nums)-1]
		}
	}

	// Free the replaced pages, the old space record, and finished log
	// chunks once no snapshot reads the previous tree.
	sp.freePages(b.freed, st.seq)
	old := st.sb.space
	sp.freePages(pageRange(old.off/pageSize, pagesFor(int(old.n))), st.seq)
	for _, c := range sp.chunks[:len(sp.chunks)-1] {
		sp.freePages(pageRange(c.start, c.n), st.seq)
	}
	sp.chunks = sp.chunks[len(sp.chunks)-1:]
	sp.closeOpen(st.seq)
	ref, err := db.writeSpace(sp)
	if err != nil {
		return err
	}

	// Make the pages and space durable, then switch superblocks.
	if err := flushDurable(db.f); err != nil {
		return err
	}
	sb := superblock{
		gen: st.gen + 1, seq: st.seq, root: root, count: uint64(st.count),
		treePages: st.sb.treePages + uint64(fresh) - uint64(len(b.freed)),
		space:     ref, logPos: st.pos, logEnd: st.end,
	}
	if _, err := db.f.WriteAt(sb.encode(), int64(superPage+sb.gen%2)*pageSize); err != nil {
		return err
	}
	if err := flushDurable(db.f); err != nil {
		return err
	}
	db.setDurable(st.seq)

	// Publish the tree.
	next := *db.stateOf(sb)
	next.pos, next.end = st.pos, st.end
	db.mtx.Lock()
	db.publish(&next)
	db.mtx.Unlock()
	db.spGen = sb.gen
	return db.punch()
}

// pageRange lists n pages from start.
func pageRange(start, n uint64) []uint64 {
	pages := make([]uint64, n)
	for i := range pages {
		pages[i] = start + uint64(i)
	}
	return pages
}

// writeSpace saves sp in newly allocated pages. The saved space marks its
// own pages in use, so allocation repeats until the encoding fits exactly
// the pages it records.
func (db *DB) writeSpace(sp *space) (extentRef, error) {
	for {
		need := pagesFor(len(sp.encode()))
		r := sp.alloc(need)
		b := sp.encode()
		if pagesFor(len(b)) != need {
			sp.addFree(r)
			continue
		}
		if _, err := db.f.WriteAt(b, int64(r.start*pageSize)); err != nil {
			return extentRef{}, err
		}
		return extentRef{off: r.start * pageSize, n: uint32(len(b)), crc: checksum(b)}, nil
	}
}

// pagesFor returns the pages that hold n bytes.
func pagesFor(n int) uint64 {
	return (uint64(n) + pageSize - 1) / pageSize
}

// relocate moves up to budget value bytes out of sparse pages, and out of
// the file's last quarter when free space below would take them, reading at most visits entries from where the last
// pass stopped. It reports whether it moved values or the walk has more
// keys to read. The caller holds the writer lock.
func (db *DB) relocate(budget int64, visits int) (bool, error) {
	// Read the published state.
	sp := db.sp
	st := db.cur.Load()

	// Start the walk at the saved key.
	it := newIterator(db, st, nil, nil, false)
	if db.relocKey != nil {
		_ = it.Seek(db.relocKey)
	} else {
		it.Next()
	}

	// Collect values in sparse pages or in the last quarter of the file.
	tail := sp.end * 3 / 4
	var moves []tentry
	var moved int64
	for ; it.Valid() && moved < budget && visits > 0; it.Next() {
		visits--
		v := it.cur
		if !v.isRef {
			continue
		}
		p := v.ref.off / pageSize
		sparse := sp.live[p] < pageSize/2
		if !sparse && (p < tail || !sp.lower(int(v.ref.n), p)) {
			continue
		}
		data, err := db.readValue(v)
		if err != nil {
			return false, err
		}
		moves = append(moves, tentry{key: bytes.Clone(it.Key()), data: data})
		moved += int64(len(data))
	}
	if err := it.Err(); err != nil {
		return false, err
	}

	// Save where the next walk resumes.
	if it.Valid() {
		db.relocKey = bytes.Clone(it.Key())
	} else {
		db.relocKey = nil
	}
	if len(moves) == 0 {
		return db.relocKey != nil, nil
	}

	// Rewrite the values: placement prefers the lowest free pages.
	sp.closeOpen(st.seq)
	return true, db.commit(st, moves, true)
}

// afterCommit runs checkpoint and compaction work. The caller holds the
// writer lock.
func (db *DB) afterCommit() error {
	// Checkpoint when the overlay has grown enough, and release the pages
	// it replaced unless a snapshot still reads them.
	st := db.cur.Load()
	if db.checkpointDue(st) {
		if err := db.checkpoint(); err != nil {
			return err
		}
		if err := db.releaseNow(); err != nil {
			return err
		}
	}

	// Move a bounded batch of values out of sparse pages once they hold a
	// quarter of the value space. Free runs need no move: they are punched,
	// so they cost file length but no disk.
	live, held := db.sp.stats()
	if db.opts.RelocateBudget > 0 && held-live >= max(1<<20, held/4) {
		_, err := db.relocate(db.opts.RelocateBudget, relocateVisits)
		return err
	}
	return nil
}

// Compact moves every value out of sparse pages and the file's tail,
// checkpoints, and releases freed space.
func (db *DB) Compact() error {
	// Take the writer lock.
	if err := db.lockWriter(); err != nil {
		return err
	}
	defer db.unlockWriter()

	// Each round moves values in one full pass of bounded batches,
	// checkpoints to free the replaced pages, and moves the log into the
	// space they leave. The second round fills the space the first
	// released and frees the old log chunk.
	for range 2 {
		db.relocKey = nil
		for first := true; first || db.relocKey != nil; first = false {
			if _, err := db.relocate(compactBatch, math.MaxInt); err != nil {
				return err
			}
		}
		if err := db.checkpoint(); err != nil {
			return err
		}
		if err := db.releaseNow(); err != nil {
			return err
		}
		if err := db.moveLog(); err != nil {
			return err
		}
	}
	return nil
}

// moveLog continues the log in the lowest free run below its current chunk,
// so the next checkpoint frees the chunk and the file can shrink. The caller
// holds the writer lock.
func (db *DB) moveLog() error {
	// Take the lowest free chunk, keeping the log where it is when the chunk
	// lies above it.
	st := db.cur.Load()
	sp := db.sp
	chunk, ok := sp.fit(extentPages, extentPages)
	if !ok {
		return nil
	}
	if chunk.start*pageSize > st.pos {
		sp.addFree(chunk)
		return nil
	}

	// Link the log to the chunk.
	sp.chunks = append(sp.chunks, chunk)
	if _, err := db.f.WriteAt(encodeLink(st.seq+1, chunk), int64(st.pos)); err != nil {
		return err
	}
	if err := flushOrdered(db.f); err != nil {
		return err
	}

	// Publish the state that continues in the chunk.
	next := *st
	next.seq++
	next.pos, next.end = chunk.start*pageSize, (chunk.start+chunk.n)*pageSize
	db.mtx.Lock()
	db.publish(&next)
	db.mtx.Unlock()
	db.spSeq = next.seq
	return nil
}

// releaseNow returns freed space no snapshot reads to the file system
// without writing a record. The next commit record carries release marks at
// least as high, so recovery repeats the release before reusing the space.
// The caller holds the writer lock.
func (db *DB) releaseNow() error {
	// Release and punch against the current state.
	st := db.cur.Load()
	r := &record{}
	if err := db.releaseSpace(st, r); err != nil {
		return err
	}
	return db.punch()
}

// Sync makes every earlier commit durable.
func (db *DB) Sync(ctx context.Context) error {
	return db.syncThrough(db.cur.Load().seq)
}

// WaitDurable returns once every earlier commit is durable, flushing the
// file when any is not. A flush from any process persists every process's
// writes.
func (db *DB) WaitDurable(ctx context.Context) error {
	return db.syncThrough(db.cur.Load().seq)
}

// syncThrough returns once commit seq is durable. Commits share flushes: one
// caller flushes for every commit published before its flush starts, and
// callers arriving meanwhile wait for it, then flush the next group if it did
// not cover them.
func (db *DB) syncThrough(seq uint64) error {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	for db.durable < seq {
		// Wait for a running flush.
		if db.flushing {
			done := db.flushed
			db.mtx.Unlock()
			<-done
			db.mtx.Lock()
			continue
		}

		// Flush every published commit.
		target := db.cur.Load().seq
		db.flushing, db.flushed = true, make(chan struct{})
		db.mtx.Unlock()
		err := flushDurable(db.f)
		db.mtx.Lock()
		db.flushing = false
		close(db.flushed)
		if err != nil {
			return err
		}
		db.durable = max(db.durable, target)
	}
	return nil
}

// Close checkpoints when this handle can take the writer lock without
// waiting, flushes, and closes the file.
func (db *DB) Close() error {
	// Stop the warm-up so its snapshot does not hold space.
	close(db.closing)
	<-db.warmDone

	// Checkpoint and release when no other process is writing.
	var err error
	if ok, lerr := db.tryLockWriter(); lerr == nil && ok {
		if err = db.catchUp(); err == nil {
			err = db.checkpoint()
		}
		if err == nil {
			err = db.releaseNow()
		}
		db.unlockWriter()
	}

	// Stop tailing, clear the reader slot, and flush.
	db.watch.stop()
	<-db.tailDone
	if _, werr := db.f.WriteAt(make([]byte, slotSize), int64(slotPage*pageSize+db.slot*slotSize)); err == nil {
		err = werr
	}
	if serr := flushDurable(db.f); err == nil {
		err = serr
	}

	// Close the watcher last: closing any descriptor drops POSIX locks.
	db.watch.close()
	if cerr := db.f.Close(); err == nil {
		err = cerr
	}
	return err
}

// tryLockWriter takes the writer lock when no process holds it.
func (db *DB) tryLockWriter() (bool, error) {
	db.wmtx.Lock()
	ok, err := lock(db.f, lockWriter, false)
	if err != nil || !ok {
		db.wmtx.Unlock()
	}
	return ok, err
}

// _ is a type assertion
var _ kvtx.Store = (*DB)(nil)
