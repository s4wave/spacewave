package s4db

import "slices"

// state is one immutable view of the database: a manifest version and the
// memtable at a record. Snapshots acquire the published state without a lock.
type state struct {
	// seq is the last record applied.
	seq uint64
	// m is the manifest version.
	m *manifest
	// mem holds the changes since the last flush.
	mem *memtable
	// flushed is the last commit the runs hold. Memtable entries at or
	// below it were dropped.
	flushed uint64
	// replay locates the first record after flushed, where an open rebuilds
	// the memtable from. The superblock a job publishes carries replay.
	replay logPoint
	// logChunks are the chunks holding the records after flushed, in log
	// order, ending with the chunk holding pos. A flush makes the chunks
	// before the one holding its replay point pending. The rest stay, since
	// the superblock it produces replays them.
	logChunks []uint64
	// inSuper is the last record whose effect on the manifest the superblock
	// already holds, so replaying it changes only the memtable.
	inSuper uint64
	// pos is the offset of the next record.
	pos uint64
	// end is the end of the log chunk holding pos.
	end uint64
	// spare is the physically backed standby log chunk.
	spare uint64
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
	// refs counts the snapshots reading the state. publish sets refs.
	refs *refCount
}

// logPoint locates a record boundary: the offset of the next record, the end
// of the chunk holding it, and the checksum and writer identity of the record
// before it, which the next record continues.
type logPoint struct {
	// pos is the offset of the next record.
	pos uint64
	// end is the end of the log chunk holding pos.
	end uint64
	// crc is the checksum of the record before pos.
	crc uint32
	// spare is the standby chunk at this record boundary.
	spare uint64
	// writer is the writer identity of the record before pos.
	writer uint64
}

// newState returns the state of superblock sb over manifest m, positioned
// to replay the memtable from the first record after sb.through, its
// memtable sized for budget bytes. Records through sb.seq apply to the
// memtable alone, since the manifest holds their effect on the table.
func newState(sb superblock, m *manifest, budget int64) *state {
	replay := logPoint{pos: sb.memPos, end: sb.memEnd, crc: sb.memCrc, writer: sb.memWriter, spare: sb.memSpare}
	return &state{
		seq:       sb.through,
		m:         m,
		mem:       newMemtable(budget),
		flushed:   sb.through,
		replay:    replay,
		logChunks: []uint64{chunkOf(sb.memPos)},
		inSuper:   sb.seq,
		pos:       sb.memPos,
		end:       sb.memEnd,
		spare:     sb.memSpare,
		crc:       sb.memCrc,
		writer:    sb.memWriter,
		logged:    sb.through,
	}
}

// point returns the log point after record seq.
func (st *state) point() logPoint {
	return logPoint{pos: st.pos, end: st.end, crc: st.crc, writer: st.writer, spare: st.spare}
}

// superblock returns the superblock describing st, without its generation
// and manifest location, which the publisher fills.
func (st *state) superblock() superblock {
	return superblock{
		version: st.m.version,
		seq:     st.seq,
		logPos:  st.pos, logEnd: st.end, logCrc: st.crc, logWriter: st.writer,
		through: st.flushed,
		memPos:  st.replay.pos, memEnd: st.replay.end, memCrc: st.replay.crc, memWriter: st.replay.writer, memSpare: st.replay.spare,
	}
}

// track starts counting st's snapshots. A state is counted from its
// publication on.
func (st *state) track() {
	st.refs = new(refCount)
}

// addReader adds d to the count of st's snapshots in stripe and returns the
// stripe's new count.
func (st *state) addReader(stripe int, d int64) int64 {
	return st.refs[stripe].n.Add(d)
}

// held reports whether any snapshot reads st.
func (st *state) held() bool {
	return st.refs.held()
}

// unflushed returns st's memtable entry of key: its newest change since the
// last flush.
func (st *state) unflushed(key []byte) (mentry, bool) {
	return st.mem.get(key)
}

// hasUnflushed reports whether st's memtable holds any change.
func (st *state) hasUnflushed() bool {
	return st.mem.len() != 0
}

// flushDue reports whether st's memtable holds changes and it, or the log
// since the last flush, passed budget.
func (st *state) flushDue(budget int64) bool {
	return st.hasUnflushed() && (st.mem.bytes > budget || st.logBytes > 4*budget)
}

// eachUnflushed calls fn with each entry of st's memtable in key order until
// fn returns false.
func (st *state) eachUnflushed(fn func(e *mentry) bool) {
	st.mem.scan(fn)
}

// unflushedLayer returns an iterator layer over st's memtable.
func (st *state) unflushedLayer() *treeIter[*mentry] {
	return st.mem.layer()
}

// changedUnder reports whether st's memtable holds a change after commit
// after to a key under prefix.
func (st *state) changedUnder(prefix []byte, after uint64) bool {
	return st.mem.changedUnder(prefix, after)
}

// chunk returns the entry of chunk c in st's table.
func (st *state) chunk(c uint64) chunkEntry {
	return st.m.table.get(c)
}

// tableEnd returns the chunk after the last one st's table holds, which is
// the file's length in chunks.
func (st *state) tableEnd() uint64 {
	return st.m.table.end()
}

// hasPending reports whether st's table holds pending or reserved chunks,
// from its tally.
func (st *state) hasPending() bool {
	t := st.m.table
	return t.count(chunkPending)+t.count(chunkReserved) != 0
}

// shrinkEnd returns the end st's table may shrink to by dropping at most n
// trailing free chunks, and whether that end is below the table's.
func (st *state) shrinkEnd(n uint64) (uint64, bool) {
	end := st.m.table.trimEnd(n)
	return end, end != st.m.table.end()
}

// stalled reports whether st's level 0 holds more runs than its stall point.
func (st *state) stalled() bool {
	return st.m.level0() > level0Stall
}

// runLevels counts st's runs per level, through the highest level that
// holds one.
func (st *state) runLevels() []int {
	var levels []int
	for _, r := range st.m.runs {
		for len(levels) <= int(r.level) {
			levels = append(levels, 0)
		}
		levels[r.level]++
	}
	return levels
}

// manifestNeed returns the chunks a job reserves for the manifest it
// publishes over st, with room for the table to grow by a quarter.
func (st *state) manifestNeed() uint64 {
	return chunksFor(st.m.table.end()*chunkEntrySize*5/4) + 1
}

// encodeManifest returns st's manifest as one checksummed record.
func (st *state) encodeManifest() []byte {
	return st.m.encode()
}

// freeRun returns the first of the n consecutive chunks an allocation from
// st's table would take, leaving st unchanged.
func (st *state) freeRun(n uint64) uint64 {
	return st.m.table.clone().alloc(n, chunkReserved, 0)
}

// reservedBy returns the chunks of cs that owner still holds reserved in
// st's table, or nil when none.
func (st *state) reservedBy(cs []uint64, owner uint64) []uint64 {
	var held []uint64
	for _, c := range cs {
		if e := st.m.table.get(c); e.kind == chunkReserved && e.owner == owner {
			held = append(held, c)
		}
	}
	return held
}

// reserve marks chunks reserved for owner's job in st's table, which the
// caller owns.
func (st *state) reserve(chunks []uint64, owner uint64) {
	st.m.table.mark(chunks, chunkEntry{kind: chunkReserved, owner: owner})
}

// allocLog takes n consecutive chunks for the log from st's table, which the
// caller owns, and returns the first.
func (st *state) allocLog(n uint64) uint64 {
	return st.m.table.alloc(n, chunkLog, 0)
}

// derive returns a copy of st the caller may change, with a fresh snapshot
// count. Its manifest, memtable and log chunks share st's storage until
// written, so deriving copies nothing that grows with the file: the log
// chunks are clipped, so an append copies them, and a flush reslices them.
func (st *state) derive() *state {
	// Copy the view over shared storage, with no snapshots counted.
	next := *st
	next.m = st.m.clone()
	next.mem = st.mem.copy()
	next.logChunks = slices.Clip(st.logChunks)
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
	st.spare = r.spare

	// A record the superblock's manifest already holds changes only the
	// memtable and the position.
	m := st.m
	if r.seq <= st.inSuper {
		switch r.kind {
		case kindLink:
			st.link(r)
		case kindCommit:
			st.applyCommit(r, false)
		case kindFlush:
			st.flush(r, budget, false)
		}
		return
	}

	// The standby belongs to the log even before a link consumes it. Historical
	// replay must not overwrite the newer manifest's chunk ownership.
	if r.spare != 0 {
		m.table.set(r.spare, chunkEntry{kind: chunkLog})
	}

	// Apply the record's kind to the manifest, table and memtable.
	switch r.kind {
	case kindLink:
		// The log continues in new chunks.
		for c := r.next; c < r.next+r.span; c++ {
			m.table.set(c, chunkEntry{kind: chunkLog})
		}
		st.link(r)
	case kindCommit:
		st.applyCommit(r, true)
	case kindReserve:
		// The job's chunks are reserved until its record, its free, or a
		// reclaim once its handle is gone.
		st.reserve(r.chunks, r.writer)
	case kindFlush:
		// The run holds the memtable through r.through. The old
		// manifest and the log chunks before the replay point are
		// pending.
		m.addRuns(r.runs, r.mchunks, r.seq)
		st.flush(r, budget, true)
	case kindMerge:
		// The outputs replace the inputs, whose chunks are pending.
		m.retireRuns(r.removed, r.seq)
		m.addRuns(r.runs, r.mchunks, r.seq)
	case kindSealed:
		// The value chunks are pending from this record, and an open one
		// closes.
		m.seal(r.chunks, r.seq)
	case kindFree:
		// The chunks are free: the writer established that no slot pins a
		// state that reads them and recovery no longer can.
		m.table.mark(r.chunks, chunkEntry{})
	case kindTruncate:
		// The table ends at r.next. The writer truncated the file in
		// the record's lock span.
		if r.next < m.table.end() {
			m.table.truncate(r.next)
		}
	}
}

// link moves the position to the chunks a link record continues in and
// adds them to the log chunks.
func (st *state) link(r *record) {
	for c := r.next; c < r.next+r.span; c++ {
		st.logChunks = append(st.logChunks, c)
	}
	st.pos, st.end = chunkStart(r.next), chunkStart(r.next+r.span)
}

// flush drops the memtable through a flush record's commit and moves the
// replay point to the record's. The log chunks before the one holding the
// replay point leave the log. With table set they become pending from this
// record, since the superblock it produces no longer replays them.
func (st *state) flush(r *record, budget int64, table bool) {
	// Drop the flushed entries and move the replay point.
	st.flushed = r.through
	st.replay = r.replay
	st.logBytes = 0
	st.mem = st.mem.dropThrough(r.through, budget)

	// Cut the log chunks before the replay point's chunk. The chunk is in
	// the chain, since the record's snapshot was taken at or after the
	// previous replay point. A chain that lacks the chunk keeps every
	// chunk.
	keep := chunkOf(r.replay.pos)
	i := slices.Index(st.logChunks, keep)
	if i < 0 {
		return
	}
	for _, c := range st.logChunks[:i] {
		if table && st.m.table.get(c).kind == chunkLog {
			st.m.table.set(c, chunkEntry{kind: chunkPending, owner: chunkLog, seq: r.seq})
		}
	}
	st.logChunks = st.logChunks[i:]
}

// addRuns adds a flush or merge record's runs and the chunks of the
// manifest written with them, keeping the runs in lookup order, and makes
// the previous manifest's chunks pending from record seq.
func (m *manifest) addRuns(runs []runMeta, mchunks []uint64, seq uint64) {
	// Add the runs into a new list, which clones of the manifest do not
	// share.
	m.version++
	for i := range runs {
		for _, c := range runs[i].chunks {
			m.table.set(c, chunkEntry{kind: chunkRun, owner: runs[i].id})
		}
	}
	m.runs = slices.Concat(m.runs, runs)
	slices.SortFunc(m.runs, lookupOrder)

	// Replace the manifest's chunks.
	for _, c := range m.mchunks {
		if m.table.get(c).kind == chunkManifest {
			m.table.set(c, chunkEntry{kind: chunkPending, owner: chunkManifest, seq: seq})
		}
	}
	for _, c := range mchunks {
		m.table.set(c, chunkEntry{kind: chunkManifest, owner: m.version})
	}
	m.mchunks = mchunks
}

// retireRuns drops the runs a merge record seq removes and makes the chunks
// each still owns pending from that record.
func (m *manifest) retireRuns(ids []uint64, seq uint64) {
	for _, in := range m.removeRuns(ids) {
		for _, c := range in.chunks {
			if e := m.table.get(c); e.kind == chunkRun && e.owner == in.id {
				m.table.set(c, chunkEntry{kind: chunkPending, owner: chunkRun, seq: seq})
			}
		}
	}
}

// seal makes the value chunks a sealed record seq lists pending from that
// record, and closes the open chunk among them.
func (m *manifest) seal(chunks []uint64, seq uint64) {
	for _, c := range chunks {
		m.table.set(c, chunkEntry{kind: chunkPending, owner: chunkValue, seq: seq})
		if c == m.hot {
			m.hot = 0
		}
		if c == m.cold {
			m.cold = 0
		}
	}
}

// applyCommit applies a commit record's changes to the memtable and, when
// table is set, its count, open chunks and value bytes to the table.
func (st *state) applyCommit(r *record, table bool) {
	// Advance the count and the open chunks.
	m := st.m
	if table {
		m.count += r.delta
		m.hot, m.cold = r.hot, r.cold
	}

	// Record each change in the memtable and its value bytes in the table,
	// then the frees, which reference whole entries, headers included.
	st.applyOps(r.ops, r.seq, table)
	if table {
		m.table.freeValues(r.frees)
	}
}

// applyOps records commit seq's key changes in the memtable and, when table
// is set, their value bytes in the table.
func (st *state) applyOps(ops []op, seq uint64, table bool) {
	for _, o := range ops {
		v := o.val
		if v.kind == valueMoved {
			// The cleaner checked under the writer lock that the
			// key still held the reference it read, so every
			// replayer applies the move. The kind only keeps the
			// move out of the flows.
			v.kind = valueRef
		}
		if v.isRef() && table {
			hdr := uint64(valueEntryHeaderLen(len(o.key), int(v.ref.n))) // #nosec G115 -- a header is a few bytes.
			st.m.table.useValue(v.ref.off-hdr, v.ref.n+uint32(hdr), seq) // #nosec G115 -- fits a chunk.
		}
		st.mem.set(mentry{key: o.key, val: v, seq: seq})
	}
}

// removeRuns drops the runs with the given ids into a new list, which clones
// of the manifest do not share, and returns the runs it dropped.
func (m *manifest) removeRuns(ids []uint64) []runMeta {
	// Index the ids.
	drop := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		drop[id] = struct{}{}
	}

	// Split the runs into those kept and those dropped.
	kept := make([]runMeta, 0, len(m.runs))
	var removed []runMeta
	for _, r := range m.runs {
		if _, ok := drop[r.id]; ok {
			removed = append(removed, r)
			continue
		}
		kept = append(kept, r)
	}
	m.runs = kept
	return removed
}
