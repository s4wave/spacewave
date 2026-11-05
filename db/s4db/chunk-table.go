package s4db

import (
	"encoding/binary"
	"iter"
	"slices"
	"sync/atomic"

	"github.com/pkg/errors"
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

// chunkKinds is the number of chunk kinds.
const chunkKinds = chunkPending + 1

// chunkPageLen is the entries in one page of a chunk table, 3 KiB of them.
const chunkPageLen = 128

// chunkDirLen is the pages in one directory of a chunk table, which covers
// chunkDirSpan chunks, 16 GiB of file.
const chunkDirLen = 128

// chunkDirSpan is the chunks one directory covers.
const chunkDirSpan = chunkPageLen * chunkDirLen

// chunkTable tracks every chunk of the file. The writer owns it; a published
// state holds an immutable copy.
//
// The entries sit in fixed pages under fixed directories that clones share:
// a clone copies one pointer per directory, and a write copies only the
// directory and page it changes, 4 KiB, never the whole table. A directory or
// page belongs to the table whose gen it carries; a write to any other copies
// it first. A table is written only before it is published, so after clone
// the source never changes.
//
// The table tallies its entries as they change, so the checks the scheduler
// makes on every wake, such as the cleaner's slack and whether any chunk is
// free or pending, read counters rather than the table.
type chunkTable struct {
	// dirs holds the pages, chunkDirLen to a directory; entries at or past n
	// are free.
	dirs []*chunkDir
	// n is the number of entries.
	n uint64
	// gen identifies the table among its clones.
	gen uint64
	// kinds counts the entries below n of each kind.
	kinds [chunkKinds]int64
	// live and used sum the live and used bytes of the value chunks.
	live, used int64
	// lowFree is at or below the lowest free chunk: no chunk below it is
	// free, so alloc starts its search there.
	lowFree uint64
}

// chunkDir is one directory of a chunk table's pages.
type chunkDir struct {
	// gen is the gen of the table that owns the directory.
	gen uint64
	// pages holds the directory's pages, nil past the table's end.
	pages [chunkDirLen]*chunkPage
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

// clone returns a copy the caller may change. It shares every directory and
// page with t, so t must not change afterwards.
func (t *chunkTable) clone() *chunkTable {
	c := *t
	c.dirs = slices.Clone(t.dirs)
	c.gen = chunkTableGen.Add(1)
	return &c
}

// end returns the chunk after the last one the table holds, which is the
// file's length in chunks.
func (t *chunkTable) end() uint64 {
	return t.n
}

// count returns the number of chunks of kind below the end.
func (t *chunkTable) count(kind uint8) int64 {
	return t.kinds[kind]
}

// get returns the entry of chunk c, free past the end.
func (t *chunkTable) get(c uint64) chunkEntry {
	if c >= t.n {
		return chunkEntry{}
	}
	return t.dirs[c/chunkDirSpan].pages[c/chunkPageLen%chunkDirLen].e[c%chunkPageLen]
}

// all yields every chunk and its entry in order.
func (t *chunkTable) all() iter.Seq2[uint64, chunkEntry] {
	return func(yield func(uint64, chunkEntry) bool) {
		for c := uint64(0); c < t.n; c++ {
			if !yield(c, t.get(c)) {
				return
			}
		}
	}
}

// tally adds entry e to the counters, or removes it with sign -1.
func (t *chunkTable) tally(e chunkEntry, sign int64) {
	t.kinds[e.kind] += sign
	if e.kind == chunkValue {
		t.live += sign * int64(e.live)
		t.used += sign * int64(e.used)
	}
}

// at returns the entry of chunk c to change, adding free entries up to it
// and copying its directory and page when a clone shares them. The caller
// removes the entry from the tally before changing it and adds it after.
func (t *chunkTable) at(c uint64) *chunkEntry {
	// Extend the table to c; the entries it adds are free.
	for uint64(len(t.dirs)) <= c/chunkDirSpan {
		t.dirs = append(t.dirs, &chunkDir{gen: t.gen})
	}
	if c >= t.n {
		t.kinds[chunkFree] += int64(c + 1 - t.n) // #nosec G115 -- a chunk count.
		t.lowFree = min(t.lowFree, t.n)
		t.n = c + 1
	}

	// Copy a shared directory, then a shared or missing page.
	d := t.dirs[c/chunkDirSpan]
	if d.gen != t.gen {
		d = &chunkDir{gen: t.gen, pages: d.pages}
		t.dirs[c/chunkDirSpan] = d
	}
	i := c / chunkPageLen % chunkDirLen
	switch p := d.pages[i]; {
	case p == nil:
		d.pages[i] = &chunkPage{gen: t.gen}
	case p.gen != t.gen:
		d.pages[i] = &chunkPage{gen: t.gen, e: p.e}
	}
	return &d.pages[i].e[c%chunkPageLen]
}

// set sets the entry of chunk c.
func (t *chunkTable) set(c uint64, e chunkEntry) {
	// Replace the entry, keeping the tally and the free hint.
	p := t.at(c)
	t.tally(*p, -1)
	*p = e
	t.tally(e, 1)
	if e.kind == chunkFree {
		t.lowFree = min(t.lowFree, c)
	}
}

// truncate drops the entries from chunk n on.
func (t *chunkTable) truncate(n uint64) {
	// Drop the entries from the tally, then clear those in the last kept
	// page, which entries past the end must keep free.
	for c := n; c < t.n; c++ {
		t.tally(t.get(c), -1)
	}
	for c := n; c < t.n && c%chunkPageLen != 0; c++ {
		*t.at(c) = chunkEntry{}
	}

	// Drop the pages past the last kept one, and the directories past its.
	pages := (n + chunkPageLen - 1) / chunkPageLen
	t.dirs = t.dirs[:(pages+chunkDirLen-1)/chunkDirLen]
	if i := pages % chunkDirLen; i != 0 {
		d := t.dirs[len(t.dirs)-1]
		if d.gen != t.gen {
			d = &chunkDir{gen: t.gen, pages: d.pages}
			t.dirs[len(t.dirs)-1] = d
		}
		clear(d.pages[i:])
	}
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
	// Find the lowest free run of n chunks from the free hint, which moves
	// up to the first free chunk the search meets; a table with no free
	// chunk extends at once, and a free run at the end extends past it.
	start, free := uint64(0), uint64(0)
	first := t.end()
	for c := max(firstChunk, t.lowFree); c < t.end() && t.kinds[chunkFree] != 0; c++ {
		if t.get(c).kind != chunkFree {
			free = 0
			continue
		}
		first = min(first, c)
		if free == 0 {
			start = c
		}
		free++
		if free == n {
			break
		}
	}
	t.lowFree = max(t.lowFree, first)
	if free == 0 {
		start = t.end()
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
		t.tally(*e, -1)
		e.kind = chunkValue
		e.seq = seq
		part := min(off+uint64(n), chunkStart(c+1)) - max(off, chunkStart(c))
		e.live += uint32(part)                                                          // #nosec G115 -- part fits a chunk.
		e.used = max(e.used, uint32(min(off+uint64(n), chunkStart(c+1))-chunkStart(c))) // #nosec G115 -- fits a chunk.
		t.tally(*e, 1)
	}
}

// appendValue moves value chunk c's cursor past n bytes and returns the
// offset of the first.
func (t *chunkTable) appendValue(c uint64, n uint32) uint64 {
	// Advance the cursor, keeping the tally.
	e := t.at(c)
	t.tally(*e, -1)
	off := chunkStart(c) + uint64(e.used)
	e.used += n
	t.tally(*e, 1)
	return off
}

// freeValue records that n value bytes at off are no longer referenced.
func (t *chunkTable) freeValue(off uint64, n uint32) {
	for c, end := chunkOf(off), chunkOf(off+uint64(n)-1); c <= end; c++ {
		e := t.at(c)
		t.tally(*e, -1)
		part := min(off+uint64(n), chunkStart(c+1)) - max(off, chunkStart(c))
		e.live -= uint32(min(uint64(e.live), part)) // #nosec G115 -- part fits a chunk.
		t.tally(*e, 1)
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
		e := chunkEntry{kind: d.kind(), owner: d.u64(), live: d.u32(), used: d.u32(), seq: d.u64()}
		if e.kind >= chunkKinds {
			d.err = errors.Wrap(ErrCorrupt, "chunk kind")
			return t
		}
		t.set(c, e)
	}
	return t
}
