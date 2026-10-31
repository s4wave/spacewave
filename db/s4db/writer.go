package s4db

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/util/csync"
)

// writer appends commits and background records for one handle. Its fields
// belong to the holder of the writer lock.
type writer struct {
	// db is the database.
	db *DB
	// mtx excludes this handle's other writers.
	mtx csync.Mutex
	// unlockMtx releases mtx while the writer lock is held.
	unlockMtx func()

	// held is set while this handle holds the writer lock. It is safe
	// without the writer lock.
	held atomic.Bool
	// waiting counts this handle's writers waiting for the writer lock. It
	// is safe without the writer lock.
	waiting atomic.Int32
}

// lock takes the writer lock and brings the handle up to date with records
// other processes appended. Waiting for another process cannot be canceled;
// ctx bounds the wait for this handle's writers.
func (w *writer) lock(ctx context.Context) error {
	// Exclude this handle's writers, then other processes, counted as
	// waiting until both are held.
	w.waiting.Add(1)
	release, err := w.mtx.Lock(ctx)
	if err != nil {
		w.waiting.Add(-1)
		return err
	}
	_, err = w.db.s.lock(lockWriter, true)
	w.waiting.Add(-1)
	if err != nil {
		release()
		return err
	}

	// Hold the lock.
	w.unlockMtx = release
	w.held.Store(true)

	// Apply what other processes wrote.
	if err := w.db.refresh(ctx); err != nil {
		w.unlock()
		return err
	}
	return nil
}

// tryLock takes the writer lock when no writer holds it, and brings the
// handle up to date as lock does: a record appended from a stale state would
// overwrite another process's records.
func (w *writer) tryLock(ctx context.Context) (bool, error) {
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
	w.held.Store(true)

	// Apply what other processes wrote.
	if err := w.db.refresh(ctx); err != nil {
		w.unlock()
		return false, err
	}
	return true, nil
}

// unlock releases the writer lock and tells other processes the file
// changed.
func (w *writer) unlock() {
	// Release other processes and wake their watchers, then this handle's
	// writers.
	w.held.Store(false)
	_ = w.db.s.unlock(lockWriter)
	w.db.watch.notify()
	release := w.unlockMtx
	w.unlockMtx = nil
	release()
}

// create writes an empty database into an empty file: the header, zero
// slots, an empty manifest in chunk 1, a log chunk at chunk 2 and the first
// superblock. The caller holds the file's writer lock.
func (w *writer) create() error {
	// Leave an existing file alone; recreate one a crash left without a
	// header.
	s := w.db.s
	size, err := s.size()
	if err != nil {
		return err
	}
	if size != 0 {
		hdr := make([]byte, pageSize)
		if _, err := s.ReadAt(hdr, headerPage*pageSize); err != nil {
			return err
		}
		if [8]byte(hdr[:8]) == magic || size > fileOff(chunkStart(3)) {
			return nil
		}
	}

	// Write an empty slot table; the header goes last, so a crash during
	// creation leaves a file without one that the next open recreates.
	if _, err := s.WriteAt(make([]byte, pageSize), slotPage*pageSize); err != nil {
		return err
	}

	// Write the manifest of an empty file: its own chunk and the log chunk.
	m := newManifest()
	*m.table.at(1) = chunkEntry{kind: chunkManifest, owner: m.version}
	*m.table.at(2) = chunkEntry{kind: chunkLog}
	mb := m.encode()
	if _, err := s.WriteAt(mb, fileOff(chunkStart(1))); err != nil {
		return err
	}

	// Point the first superblock at the manifest and the empty log, size
	// the file to its chunks, and flush.
	sb := superblock{
		gen:      1,
		version:  m.version,
		manifest: ref{off: chunkStart(1), n: uint32(len(mb)), crc: checksum(mb)}, // #nosec G115 -- an empty manifest is small.
		logPos:   chunkStart(2),
		logEnd:   chunkStart(3),
		memPos:   chunkStart(2),
		memEnd:   chunkStart(3),
	}
	if err := s.truncate(fileOff(chunkStart(3))); err != nil {
		return err
	}
	if _, err := s.WriteAt(sb.encode(), sb.page()*pageSize); err != nil {
		return err
	}
	if _, err := s.WriteAt(newHeader(w.db.opts.memtable).encode(), headerPage*pageSize); err != nil {
		return err
	}
	return s.flushDurable()
}

// valueWrite is a value awaiting its write: its entry header and bytes at
// off.
type valueWrite struct {
	// off is the file offset of the entry header.
	off uint64
	// data is the header followed by the value.
	data []byte
}

// commit appends a commit record for changes on base, writes its values,
// and publishes the new state. moves reports that the changes are the
// cleaner's moves, which the flows do not count. The caller holds the writer
// lock, and base is the published state.
func (w *writer) commit(ctx context.Context, base *state, changes []tentry, moves bool) error {
	// Start the record on a copy of the state. Placement takes chunks from
	// the copy's table, which the record's replay reproduces.
	db := w.db
	next := base.derive()
	r := &record{kind: kindCommit, writer: db.id, prevWriter: base.writer, hot: next.m.hot, cold: next.m.cold}
	var writes []valueWrite

	// A move out of an open chunk closes it, so no later value lands in the
	// chunk the cleaner is emptying.
	if moves {
		for _, c := range changes {
			switch chunkOf(c.expect) {
			case r.hot:
				r.hot = 0
			case r.cold:
				r.cold = 0
			}
		}
	}
	var flows []Flow
	if !moves {
		flows = make([]Flow, len(db.flows))
	}

	// Resolve each change against the base state: frees, count, value
	// placement, and the flow of its family.
	var userBytes int64
	for _, c := range changes {
		old, found, err := db.lookup(base, c.key)
		if err != nil {
			return err
		}
		if c.del && !found {
			continue
		}
		if moves && (!found || !old.isRef() || old.ref.off != c.expect) {
			continue
		}
		userBytes += int64(len(c.key) + len(c.data))
		if found && old.isRef() {
			hdr := uint64(valueEntryHeaderLen(len(c.key), int(old.ref.n)))                     // #nosec G115 -- a header is a few bytes.
			r.frees = append(r.frees, ref{off: old.ref.off - hdr, n: old.ref.n + uint32(hdr)}) // #nosec G115 -- fits a chunk.
			if flows != nil {
				flows[db.family(c.key)].Freed += int64(old.ref.n)
			}
		}
		switch {
		case c.del:
			r.delta--
			r.ops = append(r.ops, op{key: c.key, val: value{kind: valueDeleted}})
			continue
		case !found:
			r.delta++
		}
		v := value{kind: valueInline, inline: c.data}
		if len(c.data) > inlineMax {
			v, writes = w.place(next.m, r, c.key, c.data, writes, moves)
			if flows != nil {
				flows[db.family(c.key)].Written += int64(len(c.data))
			}
		}
		if moves {
			v.kind = valueMoved
		}
		r.ops = append(r.ops, op{key: c.key, val: v})
	}
	if len(r.ops) == 0 {
		return nil
	}

	// Find where the record goes.
	seq := base.seq + 1
	bound := uint64(recordHeader + r.bound()) // #nosec G115 -- sizes are not negative.
	pos, end, prev := base.pos, base.end, base.crc
	var link []byte
	linkPos := pos

	// Continue the log in new chunks when the record may not fit, keeping
	// room for the link at the end of every chunk.
	durable, _ := db.flush.marks()
	if pos+bound+linkSize > end {
		span := max(chunksFor(bound+linkSize), 1)
		l := &record{seq: seq, kind: kindLink, writer: db.id, prevWriter: base.writer, durable: durable, span: span}
		l.next = next.m.table.alloc(span, chunkLog, 0)
		link = l.encode(prev)
		next.apply(l, db.budget)
		seq, prev = seq+1, l.crc
		pos = chunkStart(l.next)
		r.prevWriter = db.id
	}
	r.seq = seq
	db.ownSeq.Store(seq)

	// Write the values, then the record. A tail of this handle that began
	// before the writer lock and reads the record first publishes the same
	// state.
	if err := w.writeValues(writes); err != nil {
		return err
	}
	r.durable = durable
	rec := r.encode(prev)
	if link != nil {
		if _, err := db.s.WriteAt(link, fileOff(linkPos)); err != nil {
			return err
		}
	}
	if _, err := db.s.WriteAt(rec, fileOff(pos)); err != nil {
		return err
	}
	db.written.Add(int64(len(link) + len(rec)))
	db.userBytes.Add(userBytes)

	// Publish the new state.
	next.apply(r, db.budget)
	if err := db.publish(ctx, next); err != nil {
		return err
	}
	db.addFlows(flows)
	return nil
}

// place assigns data, stored under key, a position in the open hot value
// chunk of m, or the cold chunk for a cleaner's move, taking a new chunk when
// it does not fit and a run of chunks when it exceeds one. It records the
// write and returns the reference.
func (w *writer) place(m *manifest, r *record, key, data []byte, writes []valueWrite, cold bool) (value, []valueWrite) {
	// Build the entry.
	hdr := valueEntryHeaderLen(len(key), len(data))
	entry := make([]byte, 0, hdr+len(data))
	entry = appendValueEntryHeader(entry, key, data)
	entry = append(entry, data...)
	n := uint64(len(entry))

	// Choose where it goes: its own chunks when longer than one, else the
	// open chunk, replaced when the entry does not fit.
	var off uint64
	open := &r.hot
	if cold {
		open = &r.cold
	}
	switch {
	case n > chunkSize:
		off = chunkStart(m.table.alloc(chunksFor(n), chunkValue, 0))
	default:
		if *open == 0 || uint64(m.table.at(*open).used)+n > chunkSize {
			*open = m.table.alloc(1, chunkValue, 0)
		}
		e := m.table.at(*open)
		off = chunkStart(*open) + uint64(e.used)
		e.used += uint32(n) // #nosec G115 -- n fits a chunk.
	}

	// Record the write and the reference past the header.
	writes = append(writes, valueWrite{off: off, data: entry})
	v := value{kind: valueRef, ref: ref{off: off + uint64(hdr), n: uint32(len(data)), crc: checksum(data)}} // #nosec G115 -- Set bounds values by MaxValueSize.
	return v, writes
}

// writeValues writes placed values, joining adjacent ones into one write.
func (w *writer) writeValues(writes []valueWrite) error {
	// Collect adjacent entries in buf, starting at at.
	var buf []byte
	var at uint64
	flush := func() error {
		// Write the collected entries in one call.
		if len(buf) == 0 {
			return nil
		}
		_, err := w.db.s.WriteAt(buf, fileOff(at))
		w.db.written.Add(int64(len(buf)))
		buf = buf[:0]
		return err
	}

	// Write each run when the next entry is not adjacent.
	for _, wr := range writes {
		if len(buf) != 0 && at+uint64(len(buf)) != wr.off {
			if err := flush(); err != nil {
				return err
			}
		}
		if len(buf) == 0 {
			at = wr.off
		}
		buf = append(buf, wr.data...)
	}
	return flush()
}
