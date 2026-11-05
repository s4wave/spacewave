package s4db

import (
	"encoding/binary"
	"iter"
	"slices"
	"sync/atomic"
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

// chunkPageLen is the entries in one page of a chunk table.
const chunkPageLen = 1024

// chunkTable tracks every chunk of the file. The writer owns it; a published
// state holds an immutable copy.
//
// The entries sit in fixed pages that clones share: a commit copies only the
// pages it writes, not the whole table. A page belongs to the table whose gen
// it carries; a write to any other page copies it first. A table is written
// only before it is published, so after clone the source never changes.
type chunkTable struct {
	// pages holds the entries; entries at or past n are free.
	pages []*chunkPage
	// n is the number of entries.
	n uint64
	// gen identifies the table among its clones.
	gen uint64
}

// chunkPage is one page of a chunk table.
type chunkPage struct {
	// gen is the gen of the table that owns the page.
	gen uint64
	// e holds the page's entries.
	e [chunkPageLen]chunkEntry
}

// chunkTableGen numbers tables so each clone owns no page until it writes one.
var chunkTableGen atomic.Uint64

// newChunkTable returns a table with the header chunk and no others.
func newChunkTable() *chunkTable {
	t := &chunkTable{gen: chunkTableGen.Add(1)}
	t.set(0, chunkEntry{kind: chunkLog})
	return t
}

// clone returns a copy the caller may change. It shares every page with t,
// so t must not change afterwards.
func (t *chunkTable) clone() *chunkTable {
	return &chunkTable{pages: slices.Clone(t.pages), n: t.n, gen: chunkTableGen.Add(1)}
}

// end returns the chunk after the last one the table holds, which is the
// file's length in chunks.
func (t *chunkTable) end() uint64 {
	return t.n
}

// get returns the entry of chunk c, free past the end.
func (t *chunkTable) get(c uint64) chunkEntry {
	if c >= t.n {
		return chunkEntry{}
	}
	return t.pages[c/chunkPageLen].e[c%chunkPageLen]
}

// all yields every chunk and its entry in order.
func (t *chunkTable) all() iter.Seq2[uint64, chunkEntry] {
	return func(yield func(uint64, chunkEntry) bool) {
		for c := uint64(0); c < t.n; c++ {
			if !yield(c, t.pages[c/chunkPageLen].e[c%chunkPageLen]) {
				return
			}
		}
	}
}

// at returns the entry of chunk c to change, adding free entries up to it
// and copying its page when a clone shares it.
func (t *chunkTable) at(c uint64) *chunkEntry {
	// Extend the table to c.
	for uint64(len(t.pages)) <= c/chunkPageLen {
		t.pages = append(t.pages, &chunkPage{gen: t.gen})
	}
	t.n = max(t.n, c+1)

	// Copy a shared page.
	p := t.pages[c/chunkPageLen]
	if p.gen != t.gen {
		p = &chunkPage{gen: t.gen, e: p.e}
		t.pages[c/chunkPageLen] = p
	}
	return &p.e[c%chunkPageLen]
}

// set sets the entry of chunk c.
func (t *chunkTable) set(c uint64, e chunkEntry) {
	*t.at(c) = e
}

// truncate drops the entries from chunk n on.
func (t *chunkTable) truncate(n uint64) {
	// Free the dropped entries of the last kept page, which entries past the
	// end must keep free, then drop the pages past it.
	for c := n; c < t.n && c%chunkPageLen != 0; c++ {
		t.set(c, chunkEntry{})
	}
	t.pages = t.pages[:(n+chunkPageLen-1)/chunkPageLen]
	t.n = n
}

// trimEnd returns the end the table may shrink to by dropping at most n
// trailing free chunks, never below the three chunks a new file starts with.
func (t *chunkTable) trimEnd(n uint64) uint64 {
	// Find the lowest end this call may reach.
	end := t.end()
	low := uint64(firstChunk + 3)
	if end > low+n {
		low = end - n
	}

	// Drop free chunks down to it.
	for end > low && t.get(end-1).kind == chunkFree {
		end--
	}
	return end
}

// alloc returns the lowest run of n consecutive free chunks, extending the
// table when none exists, and marks them kind with owner.
func (t *chunkTable) alloc(n uint64, kind uint8, owner uint64) uint64 {
	// Find the lowest free run of n chunks.
	start, free := uint64(0), uint64(0)
	for c := uint64(firstChunk); c < t.end(); c++ {
		if t.get(c).kind != chunkFree {
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
		t.set(c, chunkEntry{kind: kind, owner: owner})
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
	b = binary.AppendUvarint(b, t.n)
	for _, e := range t.all() {
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
	t := &chunkTable{gen: chunkTableGen.Add(1)}
	for c := range n {
		t.set(c, chunkEntry{kind: d.kind(), owner: d.u64(), live: d.u32(), used: d.u32(), seq: d.u64()})
	}
	return t
}
