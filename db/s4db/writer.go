package s4db

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// writer appends commits and background records for one handle. Its fields
// belong to the holder of the writer lock.
type writer struct {
	// db is the database.
	db *DB
	// admission grants one transaction or scheduler slice at a time.
	admission writerAdmission
	// backgroundSince starts this grant's background hold. Zero is foreground.
	backgroundSince time.Time
}

// lock admits a transaction according to its background context marker.
func (w *writer) lock(ctx context.Context) error {
	return w.take(ctx, block.IsBackground(ctx))
}

// take waits for this file's admission grant before taking its commit byte.
func (w *writer) take(ctx context.Context, background bool) error {
	// Wait for the shared grant before acquiring native writer exclusion.
	if err := w.admission.lock(ctx, background); err != nil {
		return err
	}

	// Return the grant if the native lock fails, so its successor can progress.
	if _, err := w.db.s.lock(lockWriter, true); err != nil {
		w.admission.unlock()
		return err
	}

	// Attribute the hold to this grant after native exclusion succeeds.
	w.backgroundSince = time.Time{}
	if background {
		w.backgroundSince = time.Now()
	}
	return nil
}

// tryLock admits a closing mark only while both request queues are empty.
func (w *writer) tryLock(_ context.Context) (bool, error) {
	// Optional close work never queues ahead of a real transaction.
	if !w.admission.tryLock() {
		return false, nil
	}

	// An unavailable or failed native lock also returns the optional grant.
	ok, err := w.db.s.lock(lockWriter, false)
	if err != nil || !ok {
		w.admission.unlock()
		return false, err
	}

	// Count optional marks as background holds for waiting foreground writers.
	w.backgroundSince = time.Now()
	return true, nil
}

// unlock releases the native commit byte before granting the next request.
func (w *writer) unlock() {
	// Account for every background hold, including marked transactions.
	if !w.backgroundSince.IsZero() && w.admission.waiting.Load() != 0 {
		w.db.recordWait(time.Since(w.backgroundSince))
	}

	// Native exclusion ends before the next queue grant can begin.
	_ = w.db.s.unlock(lockWriter)
	w.db.watch.notify()
	w.admission.unlock()
}

// create writes an empty database into an empty file: the header, zero
// slots, an empty manifest in chunk 1, a log chunk at chunk 2 and the first
// superblock. The caller holds the file's writer lock.
func (w *writer) create() error {
	// Leave an existing file alone. Recreate one a crash left without a
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
		if [8]byte(hdr[:8]) == magic || size > fileOff(chunkStart(4)) {
			return nil
		}
	}

	// Write an empty slot table. The header goes last, so a crash during
	// creation leaves a file without one that the next open recreates.
	if _, err := s.WriteAt(make([]byte, pageSize), slotPage*pageSize); err != nil {
		return err
	}

	// Write the manifest of an empty file: its own chunk and the log chunk.
	m := newManifest()
	m.table.set(1, chunkEntry{kind: chunkManifest, owner: m.version})
	m.mchunks = []uint64{1}
	m.table.set(2, chunkEntry{kind: chunkLog})
	m.table.set(3, chunkEntry{kind: chunkLog})
	if err := backLog(s, 2, 2); err != nil {
		return err
	}
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
		memEnd:   chunkStart(3), memSpare: 3,
	}
	if err := s.truncate(fileOff(chunkStart(4))); err != nil {
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
	r := newCommit(next.m)
	var writes []valueWrite

	// A move out of an open chunk closes it, so no later value lands in the
	// chunk the cleaner is emptying.
	if moves {
		for _, c := range changes {
			r.close(chunkOf(c.expect))
		}
	}
	var flows []Flow
	if !moves {
		flows = make([]Flow, len(db.flows))
	}

	// Look up each change's old value in the base state, with up to
	// lookupInFlight reads in flight. A cleaner's moves read around the block
	// cache, and keep only the old value's kind and reference.
	keys := make([][]byte, len(changes))
	for i := range changes {
		keys[i] = changes[i].key
	}
	olds := make([]value, len(changes))
	founds := make([]bool, len(changes))
	err := db.lookupEach(base, keys, nil, !moves, func(i int, v value, found bool) error {
		v.inline = nil
		olds[i], founds[i] = v, found
		return nil
	})
	if err != nil {
		return err
	}

	// Resolve each change against its old value: frees, count, value
	// placement, and the flow of its family.
	var userBytes int64
	for i, c := range changes {
		old, found := olds[i], founds[i]
		if c.del && !found {
			continue
		}
		if moves && (!found || !old.isRef() || old.ref.off != c.expect) {
			continue
		}
		userBytes += int64(len(c.key) + len(c.data))
		if found && old.isRef() {
			r.free(c.key, old.ref)
			if flows != nil {
				flows[db.family(c.key)].Freed += int64(old.ref.n)
			}
		}
		if c.del {
			r.remove(c.key)
			continue
		}
		// A move always places its value, so a moved value is a reference and
		// never carries an inline payload. Values in a chunk exceed inlineMax.
		v := value{kind: valueInline, inline: c.data}
		if len(c.data) > inlineMax || moves {
			v, writes = r.place(next.m, c.key, c.data, writes, moves)
			if flows != nil {
				flows[db.family(c.key)].Written += int64(len(c.data))
			}
		}
		if moves {
			v.kind = valueMoved
		}
		r.set(c.key, v, !found)
	}
	if len(r.ops) == 0 {
		return nil
	}

	// Invariant: the memtable owns every key it holds. A transaction's changes
	// are its own copies (Tx.Set) and replay owns its records, but a cleaner's
	// move keys point into the chunk it read, which a kept slice would pin
	// whole until a flush, and which the cleaner reads the next victim into.
	// A move carries no inline value, so own copies only its keys.
	if moves {
		r.own()
	}

	// Write the values and the record, and publish the new state.
	if err := w.append(ctx, base, next, r, writes); err != nil {
		return err
	}
	db.userBytes.Add(userBytes)
	db.addFlows(flows)
	return nil
}

// append places r after base in next, writes values, the link when r does
// not fit its chunk, and r, and publishes next with r applied. The caller
// holds the writer lock, base is the published state and next derives from
// it, and r's kind and body are set. Placement fills its header.
func (w *writer) append(ctx context.Context, base, next *state, r *record, writes []valueWrite) error {
	// Reserve the log and its standby before writing values or publishing.
	db := w.db
	durable, _ := db.flush.marks()
	pos, link, err := w.placeLog(base, next, r, durable)
	if err != nil {
		return err
	}
	linkPos, prev := base.pos, base.crc
	if link != nil {
		prev = next.crc
	}
	r.keepLog(next.end)

	// Write the values, then the record. A tail of this handle that began
	// before the writer lock and reads the record first publishes the same
	// state.
	if err := w.writeValues(writes); err != nil {
		return err
	}
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

	// Publish the new state.
	next.apply(r, db.budget)
	return db.publish(ctx, next)
}

// place assigns data, stored under key, a position in the commit's open hot
// value chunk, or the cold chunk for a cleaner's move, taking a new chunk
// from m when it does not fit and a run of chunks when it exceeds one
// chunk. It records the write and returns the reference.
func (r *record) place(m *manifest, key, data []byte, writes []valueWrite, cold bool) (value, []valueWrite) {
	// Build the entry.
	hdr := valueEntryHeaderLen(len(key), len(data))
	entry := make([]byte, 0, hdr+len(data))
	entry = appendValueEntryHeader(entry, key, data)
	entry = append(entry, data...)
	n := uint64(len(entry))

	// Choose where the entry goes: its own chunks when longer than one chunk,
	// else the open chunk, replaced when the entry does not fit.
	var off uint64
	open := &r.hot
	if cold {
		open = &r.cold
	}
	switch {
	case n > chunkSize:
		off = chunkStart(m.table.alloc(chunksFor(n), chunkValue, 0))
	default:
		if *open == 0 || uint64(m.table.get(*open).used)+n > chunkSize {
			*open = m.table.alloc(1, chunkValue, 0)
		}
		off = m.table.appendValue(*open, uint32(n)) // #nosec G115 -- n fits a chunk.
	}

	// Record the write and the reference past the header.
	writes = append(writes, valueWrite{off: off, data: entry})
	v := value{kind: valueRef, ref: ref{off: off + uint64(hdr), n: uint32(len(data)), crc: checksum(data)}} // #nosec G115 -- Set bounds values by MaxValueSize.
	return v, writes
}

// writeValues writes placed values, joining adjacent ones into one write.
func (w *writer) writeValues(writes []valueWrite) error {
	// A record without values writes none.
	if len(writes) == 0 {
		return nil
	}

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
