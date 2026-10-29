package s4db

import (
	"slices"
	"testing"
)

// TestSpaceChunks walks the chunk table through its allocations and frees:
// whole chunks come from the lowest free ones, index pages fill the fullest
// index chunk, a chunk freed at the end shortens the file, values append to
// the open chunk with their headers, a long value takes its own chunks, an
// emptied chunk frees only on release, drain skips chunks taken again, and
// the encoding and replay rebuild the same table.
func TestSpaceChunks(t *testing.T) {
	// A new space is one index chunk holding the fixed pages.
	sp := newSpace()
	if sp.end() != chunkPages || sp.table[0].live != firstPage {
		t.Fatalf("new space ends at %d with %d pages used", sp.end(), sp.table[0].live)
	}

	// Whole chunks extend the file, and index pages fill the fullest index
	// chunk first.
	if r := sp.takeChunks(1, chunkLog); r != chunkRun(1, 1) {
		t.Fatalf("log chunk %+v, want chunk 1", r)
	}
	sp.log = []run{chunkRun(1, 1)}
	sparse := sp.takeChunks(1, chunkIndex).start
	sp.markPages(run{start: sparse, n: 1}, true)
	if got := sp.takePages(8); !slices.Equal(got, []run{{start: firstPage, n: 8}}) {
		t.Fatalf("index pages %v, want 8 from the fixed chunk", got)
	}

	// The sparse index chunk at the end frees with its last page, and the
	// file shortens.
	sp.markPages(run{start: sparse, n: 1}, false)
	if sp.end() != 2*chunkPages || sp.free.Len() != 0 {
		t.Fatalf("end %d with %d free chunks after the last chunk freed", sp.end(), sp.free.Len())
	}

	// Values append to the open chunk after their headers until one does
	// not fit, which opens the next chunk.
	const n = 300 << 10
	var entries, offs []uint64
	for i := range 4 {
		e, off := sp.placeValue(3, n, uint64(i+1))
		entries, offs = append(entries, e), append(offs, off)
	}
	h := uint64(valueHeaderLen(3, n)) // #nosec G115 -- small.
	if entries[0] != 2*chunkSize || offs[0] != entries[0]+h || entries[1] != offs[0]+n || entries[3] != 3*chunkSize {
		t.Fatalf("entries %v offsets %v", entries, offs)
	}
	if sp.table[2].live != 3*n || sp.table[2].seq != 3 || !sp.inOpen(3) {
		t.Fatalf("chunk 2 %+v, open %+v", sp.table[2], sp.open)
	}

	// A long value takes chunks of its own and leaves the open chunk.
	long, longOff := sp.placeValue(3, 2*chunkSize, 5)
	if long != 4*chunkSize || sp.table[4].kind != chunkValue || sp.table[5].kind != chunkTail || sp.table[6].kind != chunkTail || !sp.inOpen(3) {
		t.Fatalf("long value at %d, table %v", long, kinds(sp))
	}

	// A chunk whose values die waits for release.
	for _, off := range offs[:3] {
		sp.freeValue(off, n, 6)
	}
	if sp.table[2].kind != chunkValue || len(sp.values) != 1 {
		t.Fatalf("emptied chunk freed before release: %v %v", kinds(sp), sp.values)
	}

	// Released at its tag, it frees.
	sp.release(5, 0)
	if sp.table[2].kind != chunkValue {
		t.Fatal("release below the chunk's tag freed it")
	}
	sp.release(6, 0)
	if sp.table[2].kind != chunkFree || len(sp.values) != 0 {
		t.Fatalf("released chunk not free: %v %v", kinds(sp), sp.values)
	}

	// The drain punches it; a long value freed at the end shortens the
	// file, and a chunk taken again is not punched.
	if got := sp.drain(); !slices.Equal(got, []run{chunkRun(2, 1)}) {
		t.Fatalf("drain = %v, want chunk 2", got)
	}
	sp.freeValue(longOff, 2*chunkSize, 7)
	sp.release(7, 0)
	sp.takeChunks(1, chunkIndex)
	if got := sp.drain(); len(got) != 0 || sp.end() != 4*chunkPages {
		t.Fatalf("drain = %v and end %d, want the long value's chunks cut off the end and the retaken 2 kept", got, sp.end())
	}

	// The encoding, with its record's pages in use, decodes to the same
	// table.
	r, b := sp.allocRecord()
	got, err := decodeSpace(b)
	if err != nil {
		t.Fatal(err)
	}
	if d := diffSpace(got, sp); d != "" {
		t.Fatalf("decoded space differs: %s", d)
	}
	if e := got.table[r.start/chunkPages]; e.kind != chunkIndex || e.used.count() != sp.table[r.start/chunkPages].used.count() {
		t.Fatalf("record %+v not in use in the decoded space", r)
	}

	// Replaying the placements on the same start rebuilds the same table.
	re := newSpace()
	re.claimLog(chunkRun(1, 1))
	for i, off := range offs {
		re.replayValue(off, n, uint64(i+1))
	}
	sp2 := newSpace()
	sp2.log = []run{sp2.takeChunks(1, chunkLog)}
	for i := range 4 {
		sp2.placeValue(3, n, uint64(i+1))
	}
	if d := diffSpace(re, sp2); d != "" {
		t.Fatalf("replayed space differs: %s", d)
	}
}

// kinds lists the kind of each chunk of sp.
func kinds(sp *space) []uint8 {
	out := make([]uint8, len(sp.table))
	for i, e := range sp.table {
		out[i] = e.kind
	}
	return out
}
