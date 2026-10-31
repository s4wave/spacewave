package s4db

// state is one immutable view of the database: a manifest version and the
// memtable at a record. Snapshots acquire the published state without a lock.
type state struct {
	// seq is the last record applied.
	seq uint64
	// m is the manifest version.
	m *manifest
	// mem holds the changes since the last flush.
	mem *memtable
	// flushed is the last commit the runs hold; memtable entries at or below
	// it were dropped.
	flushed uint64
	// inSuper is the last record whose effect on the manifest the superblock
	// already holds, so replaying it changes only the memtable.
	inSuper uint64
	// pos is the offset of the next record.
	pos uint64
	// end is the end of the log chunk holding pos.
	end uint64
	// crc is the checksum of record seq, which the next record continues.
	crc uint32
	// writer is the writer identity of record seq.
	writer uint64
	// logged is the highest durable mark the applied records carry, or the
	// superblock's seq when higher. Recovery checks the values of no record
	// through it, so value chunks freed at or below it may be reused.
	logged uint64
	// logBytes counts the log bytes since the last flush record this handle
	// applied, which an open would replay into the memtable.
	logBytes int64
	// refs counts the snapshots reading the state; publish sets it.
	refs *refCount
}

// newState returns the state of superblock sb over manifest m, positioned
// to replay the memtable from the first record after sb.through, its
// memtable sized for budget bytes. Records through sb.seq apply to the
// memtable alone, since the manifest holds their effect on the table.
func newState(sb superblock, m *manifest, budget int64) *state {
	return &state{
		seq:     sb.through,
		m:       m,
		mem:     newMemtable(budget),
		flushed: sb.through,
		inSuper: sb.seq,
		pos:     sb.memPos,
		end:     sb.memEnd,
		crc:     sb.memCrc,
		writer:  sb.memWriter,
		logged:  sb.through,
	}
}

// derive returns a copy of st the caller may change: its manifest and
// memtable are copied, its snapshot count is fresh.
func (st *state) derive() *state {
	// Copy the view and give it its own manifest and memtable.
	next := *st
	next.m = st.m.clone()
	next.mem = st.mem.copy()
	next.refs = nil
	return &next
}

// apply applies record r to st, whose manifest and memtable the caller owns.
func (st *state) apply(r *record, budget int64) {
	// Advance the log position and the durable mark.
	st.seq, st.crc, st.writer = r.seq, r.crc, r.writer
	st.logBytes += int64(r.size)
	st.pos += uint64(r.size) // #nosec G115 -- record sizes are not negative.
	st.logged = max(st.logged, r.durable)

	// A record the superblock's manifest already holds changes only the
	// memtable and the position.
	m := st.m
	if r.seq <= st.inSuper {
		switch r.kind {
		case kindLink:
			st.pos, st.end = chunkStart(r.next), chunkStart(r.next+r.span)
		case kindCommit:
			st.applyCommit(r, false)
		}
		return
	}

	// Apply the record's kind to the manifest, table and memtable.
	switch r.kind {
	case kindLink:
		// The log continues in new chunks.
		for c := r.next; c < r.next+r.span; c++ {
			*m.table.at(c) = chunkEntry{kind: chunkLog}
		}
		st.pos, st.end = chunkStart(r.next), chunkStart(r.next+r.span)
	case kindCommit:
		st.applyCommit(r, true)
	case kindReserve:
		// The job's chunks are reserved until its record, its free, or a
		// reclaim once its handle is gone.
		for _, c := range r.chunks {
			*m.table.at(c) = chunkEntry{kind: chunkReserved, owner: r.writer}
		}
	case kindFlush:
		// The run holds the memtable through r.through; the log before this
		// record's chunk and the old manifest are pending.
		st.addRuns(r)
		st.flushed = r.through
		st.logBytes = 0
		st.mem = st.mem.dropThrough(r.through, budget)
		for c := uint64(firstChunk); c < chunkOf(st.pos); c++ {
			if e := m.table.at(c); e.kind == chunkLog {
				*e = chunkEntry{kind: chunkPending, owner: chunkLog, seq: r.seq}
			}
		}
	case kindMerge:
		// The outputs replace the inputs, whose chunks are pending.
		for _, id := range r.removed {
			for c := range m.table.entries {
				if e := &m.table.entries[c]; e.kind == chunkRun && e.owner == id {
					*e = chunkEntry{kind: chunkPending, owner: chunkRun, seq: r.seq}
				}
			}
		}
		m.removeRuns(r.removed)
		st.addRuns(r)
	case kindSealed:
		// The value chunks are pending from this record.
		for _, c := range r.chunks {
			*m.table.at(c) = chunkEntry{kind: chunkPending, owner: chunkValue, seq: r.seq}
		}
	case kindFree:
		// The chunks are free: the writer established that no slot pins a
		// state that reads them and recovery no longer can.
		for _, c := range r.chunks {
			*m.table.at(c) = chunkEntry{}
		}
	case kindTruncate:
		// The table ends at r.next; the writer truncates the file off the
		// lock.
		if r.next < m.table.end() {
			m.table.entries = m.table.entries[:r.next]
		}
	}
}

// addRuns adds a flush or merge record's runs and manifest chunks, and
// makes the previous manifest's chunks pending.
func (st *state) addRuns(r *record) {
	// Add the runs.
	m := st.m
	m.version++
	for i := range r.runs {
		m.runs = append(m.runs, r.runs[i])
		for _, c := range r.runs[i].chunks {
			*m.table.at(c) = chunkEntry{kind: chunkRun, owner: r.runs[i].id}
		}
	}

	// Replace the manifest's chunks.
	for c := range m.table.entries {
		if e := &m.table.entries[c]; e.kind == chunkManifest {
			*e = chunkEntry{kind: chunkPending, owner: chunkManifest, seq: r.seq}
		}
	}
	for _, c := range r.mchunks {
		*m.table.at(c) = chunkEntry{kind: chunkManifest, owner: m.version}
	}
}

// applyCommit applies a commit record's changes to the memtable and, when
// table is set, its count, open chunks and value bytes to the table.
func (st *state) applyCommit(r *record, table bool) {
	// Advance the count and the open chunks.
	m := st.m
	if table {
		m.count += r.delta
		if r.hot != 0 {
			m.hot = r.hot
		}
		if r.cold != 0 {
			m.cold = r.cold
		}
	}

	// Record each change in the memtable and its value bytes in the table.
	for _, o := range r.ops {
		v := o.val
		if v.kind == valueMoved {
			// The cleaner checked under the writer lock that the key still
			// held the reference it read, so every replayer applies the
			// move; the kind only keeps it out of the flows.
			v.kind = valueRef
		}
		if v.isRef() && table {
			hdr := uint64(valueEntryHeaderLen(len(o.key), int(v.ref.n))) // #nosec G115 -- a header is a few bytes.
			m.table.useValue(v.ref.off-hdr, v.ref.n+uint32(hdr), r.seq)  // #nosec G115 -- fits a chunk.
		}
		st.mem.set(mentry{key: o.key, val: v, seq: r.seq})
	}

	// Frees name whole entries, headers included.
	if table {
		for _, f := range r.frees {
			m.table.freeValue(f.off, f.n)
		}
	}
}

// removeRuns drops the runs with the given ids.
func (m *manifest) removeRuns(ids []uint64) {
	kept := m.runs[:0]
	for _, r := range m.runs {
		drop := false
		for _, id := range ids {
			if r.id == id {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, r)
		}
	}
	clear(m.runs[len(kept):])
	m.runs = kept
}
