package s4db

import (
	"encoding/binary"
	"slices"
)

// Chunk kinds in the table.
const (
	// chunkFree holds nothing and may be allocated.
	chunkFree = 0
	// chunkLog holds log records.
	chunkLog = 1
	// chunkValue holds value entries.
	chunkValue = 2
	// chunkRun holds part of a run.
	chunkRun = 3
	// chunkManifest holds part of a manifest.
	chunkManifest = 4
	// chunkReserved was reserved by a background job and awaits its
	// record; owner is the reserving slot.
	chunkReserved = 5
	// chunkPending was freed and waits until no slot pins a state that
	// reads it and recovery can no longer read it.
	chunkPending = 6
)

// chunkEntrySize is the encoded length of a table entry.
const chunkEntrySize = 24

// chunkEntry is the table's record of one chunk.
type chunkEntry struct {
	// kind is the chunk's kind.
	kind uint8
	// owner is the run id of a run chunk, the manifest version of a manifest
	// chunk, the reserving handle's writer identity of a reserved chunk, or
	// the kind a pending chunk had, which decides its reuse rule.
	owner uint64
	// live counts a value chunk's live bytes.
	live uint32
	// used is the bytes written to a value chunk, so an open chunk's cursor
	// follows from the table.
	used uint32
	// seq is the last commit that wrote a value chunk, which dates it for
	// the cleaner, or the record that made a pending chunk pending, which the
	// release step compares with the pins and marks.
	seq uint64
}

// chunkTable tracks every chunk of the file. The writer owns it; a published
// state holds an immutable copy.
type chunkTable struct {
	// entries holds each chunk's entry; entry 0 is the header area.
	entries []chunkEntry
}

// newChunkTable returns a table with the header chunk and no others.
func newChunkTable() *chunkTable {
	return &chunkTable{entries: []chunkEntry{{kind: chunkLog}}}
}

// clone returns a copy the caller may change.
func (t *chunkTable) clone() *chunkTable {
	return &chunkTable{entries: slices.Clone(t.entries)}
}

// end returns the chunk after the last one the table holds, which is the
// file's length in chunks.
func (t *chunkTable) end() uint64 {
	return uint64(len(t.entries))
}

// at returns the entry of chunk c, adding free entries up to it.
func (t *chunkTable) at(c uint64) *chunkEntry {
	for uint64(len(t.entries)) <= c {
		t.entries = append(t.entries, chunkEntry{})
	}
	return &t.entries[c]
}

// alloc returns the lowest run of n consecutive free chunks, extending the
// table when none exists, and marks them kind with owner.
func (t *chunkTable) alloc(n uint64, kind uint8, owner uint64) uint64 {
	// Find the lowest free run of n chunks.
	start, free := uint64(0), uint64(0)
	for c := uint64(firstChunk); c < t.end(); c++ {
		if t.entries[c].kind != chunkFree {
			free = 0
			continue
		}
		if free == 0 {
			start = c
		}
		free++
		if free == n {
			break
		}
	}
	if free < n {
		if free == 0 {
			start = t.end()
		}
	}

	// Take the chunks.
	for c := start; c < start+n; c++ {
		*t.at(c) = chunkEntry{kind: kind, owner: owner}
	}
	return start
}

// useValue records n value bytes written at off by commit seq.
func (t *chunkTable) useValue(off uint64, n uint32, seq uint64) {
	// A value may span chunks; every chunk it touches is a value chunk.
	for c, end := chunkOf(off), chunkOf(off+uint64(n)-1); c <= end; c++ {
		e := t.at(c)
		e.kind = chunkValue
		e.seq = seq
		part := min(off+uint64(n), chunkStart(c+1)) - max(off, chunkStart(c))
		e.live += uint32(part)                                                          // #nosec G115 -- part fits a chunk.
		e.used = max(e.used, uint32(min(off+uint64(n), chunkStart(c+1))-chunkStart(c))) // #nosec G115 -- fits a chunk.
	}
}

// freeValue records that n value bytes at off are no longer referenced.
func (t *chunkTable) freeValue(off uint64, n uint32) {
	for c, end := chunkOf(off), chunkOf(off+uint64(n)-1); c <= end; c++ {
		e := t.at(c)
		part := min(off+uint64(n), chunkStart(c+1)) - max(off, chunkStart(c))
		e.live -= uint32(min(uint64(e.live), part)) // #nosec G115 -- part fits a chunk.
	}
}

// encode appends the table to b.
func (t *chunkTable) encode(b []byte) []byte {
	b = binary.AppendUvarint(b, uint64(len(t.entries)))
	for _, e := range t.entries {
		b = append(b, e.kind)
		b = binary.LittleEndian.AppendUint64(b, e.owner)
		b = binary.LittleEndian.AppendUint32(b, e.live)
		b = binary.LittleEndian.AppendUint32(b, e.used)
		b = binary.LittleEndian.AppendUint64(b, e.seq)
	}
	return b
}

// decodeChunkTable reads a table.
func decodeChunkTable(d *decoder) *chunkTable {
	n := d.count()
	t := &chunkTable{entries: make([]chunkEntry, n)}
	for i := range t.entries {
		t.entries[i] = chunkEntry{kind: d.kind(), owner: d.u64(), live: d.u32(), used: d.u32(), seq: d.u64()}
	}
	return t
}
