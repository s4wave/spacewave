package s4db

import (
	"bytes"
	"context"
	"math"

	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
)

// relocateVisits bounds the entries one incremental relocation pass reads, so
// a commit's compaction work does not grow with the key count.
const relocateVisits = 4096

// compactBatch bounds the value bytes Compact holds in memory and commits
// at once.
const compactBatch = 64 << 20

// writer appends commits, checkpoints, and moves values for one handle. Its
// fields belong to the holder of the writer lock.
type writer struct {
	// db is the database.
	db *DB
	// mtx excludes this handle's other writers.
	mtx csync.Mutex
	// unlockMtx releases mtx while the writer lock is held.
	unlockMtx func()
	// sp is the space state, current through spSeq and spGen.
	sp *space
	// spSeq is the commit sp reflects.
	spSeq uint64
	// spGen is the checkpoint generation sp reflects.
	spGen uint64
	// fileEnd is the file length in pages.
	fileEnd uint64
	// relocKey is where the next compaction pass resumes.
	relocKey []byte
}

// lock takes the writer lock and brings the writer up to date with records
// other processes appended. Waiting for another process cannot be
// canceled; ctx bounds the wait for this handle's writers.
func (w *writer) lock(ctx context.Context) error {
	// Exclude this handle's writers, then other processes.
	release, err := w.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	if _, err := w.db.s.lock(lockWriter, true); err != nil {
		release()
		return err
	}
	w.unlockMtx = release

	// Apply what other processes wrote.
	if err := w.catchUp(ctx); err != nil {
		w.unlock(err)
		return err
	}
	return nil
}

// tryLock takes the writer lock when no writer holds it, without catching
// up.
func (w *writer) tryLock() (bool, error) {
	// Exclude this handle's writers, then other processes, without
	// waiting.
	release, ok := w.mtx.TryLock()
	if !ok {
		return false, nil
	}
	ok, err := w.db.s.lock(lockWriter, false)
	if err != nil || !ok {
		release()
		return false, err
	}
	w.unlockMtx = release
	return true, nil
}

// unlock releases the writer lock and tells other processes the file
// changed. err is the result of the work done under the lock: a failed
// step may have changed the space for writes the log does not hold, so
// the space is dropped and the next lock rebuilds it from the log.
func (w *writer) unlock(err error) {
	// Drop a space a failed step may have left ahead of the log.
	if err != nil {
		w.sp = nil
	}

	// Release other processes and wake their watchers, then this handle's
	// writers.
	_ = w.db.s.unlock(lockWriter)
	w.db.watch.notify()
	release := w.unlockMtx
	w.unlockMtx = nil
	release()
}

// create writes an empty database into an empty file. The caller holds the
// file's writer lock.
func (w *writer) create() error {
	// Leave an existing file alone.
	s := w.db.s
	if size, err := s.size(); err != nil || size != 0 {
		return err
	}

	// Write the header and an empty slot table.
	if _, err := s.WriteAt(newHeader(w.db.opts.InlineMax).encode(), headerPage*pageSize); err != nil {
		return err
	}
	if _, err := s.WriteAt(make([]byte, pageSize), slotPage*pageSize); err != nil {
		return err
	}

	// Allocate the first log chunk and save the space.
	sp := newSpace()
	chunk := sp.alloc(extentPages)
	sp.chunks = []run{chunk}
	ref, err := w.writeSpace(sp)
	if err != nil {
		return err
	}

	// Point the first superblock at the empty log and flush.
	sb := superblock{gen: 1, space: ref, logPos: chunk.start * pageSize, logEnd: chunk.end() * pageSize}
	if err := s.truncate(pageOff(sp.end)); err != nil {
		return err
	}
	if _, err := s.WriteAt(sb.encode(), sb.page()*pageSize); err != nil {
		return err
	}
	return s.flushDurable()
}

// catchUp applies new records and rebuilds the space when another process
// wrote since this handle last did. The caller holds the writer lock.
func (w *writer) catchUp(ctx context.Context) error {
	// Apply the new records and keep the space when it is still current.
	db := w.db
	st, _, err := db.tail(db.cur.Load(), false)
	if err != nil {
		return err
	}
	if err := db.publish(ctx, st); err != nil {
		return err
	}
	st = db.cur.Load()
	if w.sp != nil && w.spSeq == st.seq && w.spGen == st.gen {
		return nil
	}
	if err := w.replaySpace(st); err != nil {
		return err
	}

	// Flush whatever the previous writer left unflushed, so this writer's
	// durability marks hold.
	return db.flush.syncThrough(ctx, st.seq, st.ckpt)
}

// replaySpace loads the space of st's checkpoint and replays the records
// after it.
func (w *writer) replaySpace(st *state) error {
	// Load the checkpoint's space.
	s := w.db.s
	ref := st.sb.space
	b := make([]byte, ref.n)
	if _, err := s.ReadAt(b, fileOff(ref.off)); err != nil {
		return err
	}
	if checksum(b) != ref.crc {
		return errors.Wrap(ErrCorrupt, "space checksum mismatch")
	}
	sp, err := decodeSpace(b)
	if err != nil {
		return err
	}

	// Replay the space changes of the records after it.
	lr := newLogReader(s, st.sb.logPos, st.sb.logEnd, st.sb.seq)
	for lr.seq <= st.seq {
		r, ok := lr.next()
		if !ok {
			return errors.Wrapf(ErrCorrupt, "log ends before record %d", st.seq)
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
		for _, fr := range r.frees {
			sp.freeValue(fr.off, int(fr.n), r.seq)
		}
	}

	// Adopt the space; its writer already punched what it released.
	sp.drain()
	size, err := s.size()
	if err != nil {
		return err
	}
	w.sp, w.spSeq, w.spGen = sp, st.seq, st.gen
	w.fileEnd = uint64(size) / pageSize // #nosec G115 -- file sizes are not negative.
	return nil
}

// commit appends a commit record for changes on base, writes its values,
// and publishes the new state. The caller holds the writer lock.
func (w *writer) commit(ctx context.Context, base *state, changes []tentry, ordered bool) error {
	// Resolve each change against the base state: frees, count, and value
	// placement.
	db, sp := w.db, w.sp
	r := &record{kind: kindCommit}
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
			r.frees = append(r.frees, old.ref)
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
			v = value{isRef: true, ref: extentRef{n: uint32(len(c.data)), crc: checksum(c.data)}} // #nosec G115 -- Set bounds values by MaxValueSize.
			writes = append(writes, valueWrite{op: len(r.ops), data: c.data})
		}
		r.ops = append(r.ops, op{key: c.key, val: v})
	}
	if len(r.ops) == 0 {
		return nil
	}

	// Refuse a record its length field cannot hold.
	size := r.bound() + linkSize
	if int64(size) > maxRecord {
		return ErrTxTooLarge
	}

	// Continue the log in a new chunk when the record may not fit.
	bound := uint64(size) // #nosec G115 -- sizes are not negative.
	seq := base.seq + 1
	pos, end := base.pos, base.end
	var link []byte
	linkPos := pos
	if pos+bound > end {
		chunk := sp.alloc(max(extentPages, pagesFor(size)))
		sp.chunks = append(sp.chunks, chunk)
		link = newLink(seq, chunk).encode()
		seq++
		pos, end = chunk.start*pageSize, chunk.end()*pageSize
	}
	r.seq = seq

	// Place values, then free replaced ones and release what no snapshot
	// reads.
	for _, wr := range writes {
		ref := &r.ops[wr.op].val.ref
		ref.off = sp.placeValue(len(wr.data), seq)
	}
	for _, fr := range r.frees {
		sp.freeValue(fr.off, int(fr.n), seq)
	}
	if err := w.releaseSpace(ctx, base, r); err != nil {
		return err
	}

	// Write the values, then the record. A tail of this handle that reads
	// the record first publishes the same state.
	if err := w.writeValues(r, writes); err != nil {
		return err
	}
	r.durable, _ = db.flush.marks()
	rec := r.encode()
	if err := w.writeRecord(link, linkPos, rec, pos); err != nil {
		return err
	}

	// Publish the new state.
	next := db.derive(base)
	next.apply(r)
	next.pos, next.end = pos+uint64(len(rec)), end
	if err := db.publish(ctx, next); err != nil {
		return err
	}

	// Order the next commit's writes after this one's and release space.
	// A durable commit flushes after the writer lock is released.
	if ordered {
		if err := db.s.flushOrdered(); err != nil {
			return err
		}
		db.flush.ordered()
	}
	if err := w.punch(); err != nil {
		return err
	}

	// The space now reflects the record.
	w.spSeq = seq
	return nil
}

// writeRecord writes a commit record at pos, after the link that leads to
// its chunk when link is set.
func (w *writer) writeRecord(link []byte, linkPos uint64, rec []byte, pos uint64) error {
	if link != nil {
		if _, err := w.db.s.WriteAt(link, fileOff(linkPos)); err != nil {
			return err
		}
	}
	_, err := w.db.s.WriteAt(rec, fileOff(pos))
	return err
}

// writeValues writes placed values, joining adjacent ones into one write.
func (w *writer) writeValues(r *record, writes []valueWrite) error {
	// Collect adjacent values in buf, starting at at.
	var buf []byte
	var at uint64
	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		_, err := w.db.s.WriteAt(buf, fileOff(at))
		buf = buf[:0]
		return err
	}

	// Write each run when the next value is not adjacent.
	for _, wr := range writes {
		off := r.ops[wr.op].val.ref.off
		if len(buf) != 0 && at+uint64(len(buf)) != off {
			if err := flush(); err != nil {
				return err
			}
		}
		if len(buf) == 0 {
			at = off
		}
		buf = append(buf, wr.data...)
	}
	return flush()
}

// releaseSpace returns freed runs no snapshot reads to free space and
// records the space's release marks in r. The marks only rise, so replaying
// any record repeats every release the writer made before it.
func (w *writer) releaseSpace(ctx context.Context, base *state, r *record) error {
	// Bound the release by durability and this handle's snapshots, and skip
	// reading the slots when nothing pending falls under that bound.
	sp := w.sp
	defer func() { r.released = sp.marks }()
	durable, ckptDurable := w.db.flush.marks()
	local := w.db.localPin()
	seq := min(local.seq, durable)
	ckpt := min(local.ckpt, base.ckpt, ckptDurable)
	if !sp.due(seq, ckpt) {
		return nil
	}

	// Lower the bound by every other process's snapshots and release value
	// runs and pages below it.
	p, err := w.db.minPin()
	if err != nil {
		return err
	}
	sp.release(min(p.seq, seq), min(p.ckpt, ckpt))
	return nil
}

// punch deallocates released runs and shrinks the file to the space end.
func (w *writer) punch() error {
	// Deallocate released runs below the end.
	db, sp := w.db, w.sp
	for _, r := range sp.drain() {
		if r.start >= sp.end {
			continue
		}
		r.n = min(r.n, sp.end-r.start)
		db.p.cache.drop(r)
		if err := db.s.punch(pageOff(r.start), pageOff(r.n)); err != nil {
			return err
		}
	}

	// Cut the file at the end.
	if sp.end < w.fileEnd {
		db.p.cache.drop(run{start: sp.end, n: w.fileEnd - sp.end})
		if err := db.s.truncate(pageOff(sp.end)); err != nil {
			return err
		}
	}
	w.fileEnd = sp.end
	return nil
}

// checkpoint writes the overlay into the tree and saves a superblock. The
// caller holds the writer lock.
func (w *writer) checkpoint(ctx context.Context) error {
	// Skip when no commit follows the last checkpoint.
	db, sp := w.db, w.sp
	st := db.cur.Load()
	if st.seq == st.ckpt {
		return nil
	}

	// Apply the overlay to the tree.
	changes := make([]change, 0, st.overlay.Len())
	st.overlay.Scan(func(it overlayItem) bool {
		e := it.e
		changes = append(changes, change{key: e.key, del: e.del, val: e.val})
		return true
	})
	b := newBuilder(db.p)
	top, err := b.build(st.root, changes)
	if err != nil {
		return err
	}

	// Write the new pages into one run; the root is the last of them, or
	// an existing page when nothing above it changed.
	var root uint64
	if top != nil {
		root = top.page
	}
	fresh := b.reachable(top)
	if fresh != 0 {
		r := sp.alloc(uint64(fresh)) // #nosec G115 -- counts are not negative.
		nodes, err := b.place(top, r.start, func(buf []byte, page uint64) error {
			_, err := db.s.WriteAt(buf, pageOff(page))
			return err
		})
		if err != nil {
			return err
		}
		for i, n := range nodes {
			db.p.cache.put(r.start+uint64(i), n)
		}
		root = r.start + uint64(len(nodes)) - 1
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

	// Close the open value extent and save the space.
	sp.closeOpen(st.seq)
	ref, err := w.writeSpace(sp)
	if err != nil {
		return err
	}

	// Order the pages and space before the superblock that names them. A
	// durable barrier also makes the records and the previous superblock
	// durable.
	prev := db.flush.newest()
	durable, err := db.s.flushBarrier()
	if err != nil {
		return err
	}
	if durable {
		db.flush.setDurable(st.seq, prev)
	}

	// Switch superblocks. The new one becomes durable with a later flush,
	// which releases the pages it replaced.
	sb := superblock{
		gen:       st.gen + 1,
		seq:       st.seq,
		root:      root,
		count:     uint64(st.count),                                       // #nosec G115 -- key counts are not negative.
		treePages: st.sb.treePages + uint64(fresh) - uint64(len(b.freed)), // #nosec G115 -- counts are not negative.
		space:     ref,
		logPos:    st.pos,
		logEnd:    st.end,
	}
	if _, err := db.s.WriteAt(sb.encode(), sb.page()*pageSize); err != nil {
		return err
	}

	// Publish the tree.
	if err := db.publish(ctx, newState(sb, db.opts.checkpointLimit(sb))); err != nil {
		return err
	}
	w.spGen = sb.gen
	return w.punch()
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
func (w *writer) writeSpace(sp *space) (extentRef, error) {
	for {
		need := pagesFor(len(sp.encode()))
		r := sp.alloc(need)
		b := sp.encode()
		if pagesFor(len(b)) != need {
			sp.addFree(r)
			continue
		}
		if _, err := w.db.s.WriteAt(b, pageOff(r.start)); err != nil {
			return extentRef{}, err
		}
		return extentRef{off: r.start * pageSize, n: uint32(len(b)), crc: checksum(b)}, nil // #nosec G115 -- the space record is far below 4 GiB.
	}
}

// relocate moves up to budget value bytes out of sparse pages, and out of
// the file's last quarter when free space below would take them, reading at
// most visits entries from where the last pass stopped. The caller holds
// the writer lock.
func (w *writer) relocate(ctx context.Context, budget int64, visits int) error {
	// Start the walk at the saved key.
	db, sp := w.db, w.sp
	st := db.cur.Load()
	it := newIterator(db.p, st, nil, nil, false)
	if w.relocKey != nil {
		_ = it.Seek(w.relocKey)
	} else {
		it.Next()
	}

	// Collect values in sparse pages or in the last quarter of the file.
	tail := sp.end * 3 / 4
	var moves []tentry
	var moved int64
	for ; it.Valid() && moved < budget && visits > 0; it.Next() {
		visits--
		v := it.cur.val
		if !v.isRef {
			continue
		}
		p := v.ref.off / pageSize
		sparse := sp.live[p] < pageSize/2
		if !sparse && (p < tail || !sp.lower(int(v.ref.n), p)) {
			continue
		}
		data, err := db.p.readValue(v)
		if err != nil {
			return err
		}
		moves = append(moves, tentry{key: bytes.Clone(it.Key()), data: data})
		moved += int64(len(data))
	}
	if err := it.Err(); err != nil {
		return err
	}

	// Save where the next walk resumes.
	w.relocKey = nil
	if it.Valid() {
		w.relocKey = bytes.Clone(it.Key())
	}
	if len(moves) == 0 {
		return nil
	}

	// Rewrite the values: placement prefers the lowest free pages.
	sp.closeOpen(st.seq)
	return w.commit(ctx, st, moves, true)
}

// afterCommit runs checkpoint and compaction work. The caller holds the
// writer lock.
func (w *writer) afterCommit(ctx context.Context) error {
	// Checkpoint when the overlay has grown enough, and release the pages
	// it replaced unless a snapshot still reads them.
	db := w.db
	st := db.cur.Load()
	if st.overlayBytes > db.opts.checkpointLimit(st.sb) {
		if err := w.checkpoint(ctx); err != nil {
			return err
		}
		if err := w.releaseNow(ctx); err != nil {
			return err
		}
	}

	// Move a bounded batch of values out of sparse pages once they hold a
	// quarter of the value space. Free runs need no move: they are punched,
	// so they cost file length but no disk.
	live, held := w.sp.stats()
	if db.opts.RelocateBudget > 0 && held-live >= max(1<<20, held/4) {
		return w.relocate(ctx, db.opts.RelocateBudget, relocateVisits)
	}
	return nil
}

// compact moves every value out of sparse pages and the file's tail,
// checkpoints, and releases freed space. The caller holds the writer lock.
func (w *writer) compact(ctx context.Context) error {
	// Each round moves values in one full pass of bounded batches,
	// checkpoints to free the replaced pages, and moves the log into the
	// space they leave. The second round fills the space the first
	// released and frees the old log chunk.
	for range 2 {
		w.relocKey = nil
		for first := true; first || w.relocKey != nil; first = false {
			if err := w.relocate(ctx, compactBatch, math.MaxInt); err != nil {
				return err
			}
		}
		if err := w.checkpointDurable(ctx); err != nil {
			return err
		}
		if err := w.releaseNow(ctx); err != nil {
			return err
		}
		if err := w.moveLog(ctx); err != nil {
			return err
		}
	}
	return nil
}

// moveLog continues the log in the lowest free run below its current chunk,
// so the next checkpoint frees the chunk and the file can shrink. The caller
// holds the writer lock.
func (w *writer) moveLog(ctx context.Context) error {
	// Take the lowest free chunk, keeping the log where it is when the chunk
	// lies above it.
	db, sp := w.db, w.sp
	st := db.cur.Load()
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
	if _, err := db.s.WriteAt(newLink(st.seq+1, chunk).encode(), fileOff(st.pos)); err != nil {
		return err
	}
	if err := db.s.flushOrdered(); err != nil {
		return err
	}

	// Publish the state that continues in the chunk.
	next := *st
	next.seq++
	next.pos, next.end = chunk.start*pageSize, chunk.end()*pageSize
	next.refs = nil
	if err := db.publish(ctx, &next); err != nil {
		return err
	}
	w.spSeq = next.seq
	return nil
}

// checkpointDurable checkpoints and flushes, so releaseNow can return the
// pages the checkpoint replaced. The caller holds the writer lock.
func (w *writer) checkpointDurable(ctx context.Context) error {
	if err := w.checkpoint(ctx); err != nil {
		return err
	}
	st := w.db.cur.Load()
	return w.db.flush.syncThrough(ctx, st.seq, st.ckpt)
}

// releaseNow returns freed space no snapshot reads to the file system
// without writing a record. The next commit record carries release marks at
// least as high, so recovery repeats the release before reusing the space.
// The caller holds the writer lock.
func (w *writer) releaseNow(ctx context.Context) error {
	if err := w.releaseSpace(ctx, w.db.cur.Load(), &record{}); err != nil {
		return err
	}
	return w.punch()
}
