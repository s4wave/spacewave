package s4db

import (
	"cmp"
	"encoding/binary"
	"math/bits"
	"slices"

	"github.com/pkg/errors"
	"github.com/tidwall/btree"
)

// run is a range of pages.
type run struct {
	// start is the first page.
	start uint64
	// n is the page count.
	n uint64
}

// end returns the page after r.
func (r run) end() uint64 {
	return r.start + r.n
}

// pending is a run freed at tag that stays allocated until no snapshot can
// still read it.
type pending struct {
	run
	// tag is the commit sequence that freed a value chunk, or the checkpoint
	// that freed index pages or a log chunk.
	tag uint64
}

// Chunk kinds. The file is a sequence of chunks, and each chunk below the
// end is in exactly one of these states.
const (
	// chunkFree holds nothing. A free chunk below the end has been or will
	// be punched; the last chunk is never free.
	chunkFree = iota
	// chunkHeld is free but kept out of allocation while a background punch
	// deallocates it. A saved space records it as free.
	chunkHeld
	// chunkLog holds log records.
	chunkLog
	// chunkIndex holds index pages and space records, a page at a time.
	chunkIndex
	// chunkValue holds values, each after its header, appended from the
	// chunk's start.
	chunkValue
	// chunkTail continues a value that starts in an earlier chunk.
	chunkTail
)

// pageMap marks the used pages of an index chunk.
type pageMap [chunkPages / 64]uint64

// count returns the used pages.
func (m *pageMap) count() uint32 {
	n := 0
	for _, w := range m {
		n += bits.OnesCount64(w)
	}
	return uint32(n) // #nosec G115 -- at most chunkPages.
}

// set marks pages [lo, hi) of the chunk used or unused and returns the change
// in used pages.
func (m *pageMap) set(lo, hi uint64, used bool) int {
	d := 0
	for p := lo; p < hi; p++ {
		w, bit := &m[p/64], uint64(1)<<(p%64)
		if (*w&bit != 0) == used {
			continue
		}
		*w ^= bit
		d++
	}
	if !used {
		d = -d
	}
	return d
}

// span returns the first run of at least least unused pages, at most want
// long, reporting false when none is that long.
func (m *pageMap) span(least, want uint64) (lo, hi uint64, ok bool) {
	for p := uint64(0); p < chunkPages; {
		// Skip used pages.
		if m[p/64]&(1<<(p%64)) != 0 {
			p++
			continue
		}

		// Measure the unused run from p.
		q := p
		for q < chunkPages && q-p < want && m[q/64]&(1<<(q%64)) == 0 {
			q++
		}
		if q-p >= least {
			return p, q, true
		}
		p = q
	}
	return 0, 0, false
}

// chunk is a chunk's entry in the chunk table.
type chunk struct {
	// kind is the chunk's state.
	kind uint8
	// live counts the live value bytes of a value or tail chunk, or the used
	// pages of an index chunk.
	live uint32
	// seq is the last commit that placed a value in a value or tail chunk,
	// which dates its contents for cleaning.
	seq uint64
	// used marks the used pages of an index chunk.
	used pageMap
}

// fullness orders the index chunks that have unused pages, fullest first.
type fullness struct {
	// used is the chunk's used pages.
	used uint32
	// chunk is the chunk number.
	chunk uint64
}

// lessFullness orders fuller chunks first, then lower ones.
func lessFullness(a, b fullness) bool {
	if a.used != b.used {
		return a.used > b.used
	}
	return a.chunk < b.chunk
}

// Value streams. New values and the values the cleaner moves fill separate
// open chunks: moved values are old and cold, so chunks of them stay full
// and need cleaning least.
const (
	// streamNew takes the values commits write.
	streamNew = iota
	// streamMoved takes the values the cleaner moves.
	streamMoved
	// streams is the number of streams.
	streams
)

// openChunks is the number of value chunks each stream keeps open. A value
// goes to the open chunk it fits best, so the room a long value leaves at a
// chunk's end takes the short values after it.
const openChunks = 4

// extent is an open value chunk, as file offsets.
type extent struct {
	// start is the chunk's first byte.
	start uint64
	// cursor is where the next value's header goes.
	cursor uint64
	// end is the byte after the chunk, or zero when none is open.
	end uint64
}

// space is the chunk table: what each chunk of the file holds. The writer
// owns it, and a saved space plus the log records after it rebuild it in
// every process.
//
// Invariants:
//   - Every allocation takes whole chunks, except index pages, which fill the
//     unused pages of index chunks. So free space is whole chunks, and a free
//     chunk is never cut into shorter runs.
//   - A chunk is free only when nothing the current state or any snapshot
//     reads lies in it: a value or log chunk frees when its pending run is
//     released, an index chunk when its last used page is released or its
//     last reservation returns unused.
//   - free holds exactly the free chunks below the end, and partial exactly
//     the index chunks with unused pages. The last chunk is never free: the
//     end falls past free chunks as they free.
//   - A value chunk is written only at its stream's open chunk's cursor, so
//     its entries run from its start without gaps, and bytes below the
//     cursor never change while the chunk is in use.
//   - A value or tail chunk's live is the sum of the bytes in it of the
//     values live keys refer to, and an index chunk's live counts its used
//     pages. A value or tail chunk with no live bytes is pending, and an
//     open chunk closes when its last live byte is freed.
//   - The table decodeSpace and replay derive from the file equals the
//     writer's, except for reservations, which no saved tree names, so a
//     reloaded space frees them.
type space struct {
	// table holds an entry for each chunk below the end.
	table []chunk
	// free holds the free chunk numbers.
	free *btree.BTreeG[uint64]
	// partial holds the index chunks with unused pages, fullest first.
	partial *btree.BTreeG[fullness]
	// liveBytes is the total live value bytes.
	liveBytes int64
	// values holds pending value and tail chunks in tag order.
	values []pending
	// pages holds pending index pages and log chunks in tag order.
	pages []pending
	// log lists the log chunks in log order.
	log []run
	// open is each stream's value chunks taking its values.
	open [streams][openChunks]extent
	// released collects chunks freed since the caller last drained it, for
	// hole punching.
	released []run
	// releasedPages counts the pages of released.
	releasedPages uint64
	// marks are the highest release marks applied: value chunks through
	// marks.seq and page runs through marks.ckpt are free.
	marks pin
	// reserved lists the index pages checkpoints are writing new pages into.
	reserved []run
	// evac is the index chunk the cleaner empties, which index pages and
	// space records do not take, or zero for none: the first chunk holds the
	// fixed pages and is never emptied. A reloaded space forgets it.
	evac uint64
}

// newSpace returns the space of a new file: the first chunk, an index chunk
// whose first pages hold the header, the slots, and the superblocks.
func newSpace() *space {
	// Start with the first chunk, holding the fixed pages.
	s := &space{
		free:    btree.NewBTreeGOptions(func(a, b uint64) bool { return a < b }, btree.Options{NoLocks: true}),
		partial: btree.NewBTreeGOptions(lessFullness, btree.Options{NoLocks: true}),
	}
	s.table = []chunk{{kind: chunkIndex}}
	s.partial.Set(fullness{chunk: 0})
	s.markPages(run{n: firstPage}, true)
	return s
}

// end returns the file length in pages.
func (s *space) end() uint64 {
	return uint64(len(s.table)) * chunkPages
}

// chunkRun returns the pages of n chunks from chunk c.
func chunkRun(c, n uint64) run {
	return run{start: c * chunkPages, n: n * chunkPages}
}

// chunksFor returns the chunks that hold n pages.
func chunksFor(pages uint64) uint64 {
	return (pages + chunkPages - 1) / chunkPages
}

// lowestFree returns the first of the lowest n consecutive free chunks.
func (s *space) lowestFree(n uint64) (uint64, bool) {
	// One chunk is the lowest free one.
	if n == 1 {
		return s.free.Min()
	}

	// Else find the lowest run of n.
	var start, length uint64
	found := false
	s.free.Scan(func(c uint64) bool {
		if length == 0 || c != start+length {
			start, length = c, 0
		}
		length++
		found = length == n
		return !found
	})
	return start, found
}

// takeChunks takes n consecutive chunks for kind: the lowest free ones that
// hold them, else past the end.
func (s *space) takeChunks(n uint64, kind uint8) run {
	start, ok := s.lowestFree(n)
	if !ok {
		start = uint64(len(s.table))
	}
	for c := start; c < start+n; c++ {
		s.setKind(c, kind)
	}
	return chunkRun(start, n)
}

// setKind makes chunk c an empty chunk of kind, taking it out of the free or
// partial chunks and growing the table to cover it.
func (s *space) setKind(c uint64, kind uint8) {
	// Grow the table, freeing the chunks between the end and c.
	for uint64(len(s.table)) <= c {
		if n := uint64(len(s.table)); n < c {
			s.free.Set(n)
		}
		s.table = append(s.table, chunk{})
	}

	// Leave the old state and enter the new one.
	e := &s.table[c]
	switch e.kind {
	case chunkFree:
		s.free.Delete(c)
	case chunkIndex:
		s.partial.Delete(fullness{used: e.live, chunk: c})
	}
	*e = chunk{kind: kind}
	if kind == chunkIndex {
		s.partial.Set(fullness{chunk: c})
	}
}

// freeChunk makes chunk c free, queuing it for hole punching when punch is
// set, and lowers the end past the free chunks that end the file.
func (s *space) freeChunk(c uint64, punch bool) {
	// Leave the old state.
	e := &s.table[c]
	if e.kind == chunkIndex {
		s.partial.Delete(fullness{used: e.live, chunk: c})
	}
	if c == s.evac {
		s.evac = 0
	}
	*e = chunk{}

	// Lower the end, or queue the chunk.
	if c+1 == uint64(len(s.table)) {
		s.table = s.table[:c]
		for l := len(s.table); l > 0 && s.table[l-1].kind == chunkFree; l-- {
			s.free.Delete(uint64(l - 1))
			s.table = s.table[:l-1]
		}
		return
	}
	s.free.Set(c)
	if punch {
		s.released = append(s.released, chunkRun(c, 1))
		s.releasedPages += chunkPages
	}
}

// markPages marks the pages of r, which lie in index chunks, used or unused,
// freeing an index chunk whose last page goes unused.
func (s *space) markPages(r run, used bool) {
	for p := r.start; p < r.end(); {
		// Change the pages of r in this chunk.
		c := p / chunkPages
		hi := min(r.end(), (c+1)*chunkPages)
		e := &s.table[c]
		old := e.live
		e.live = uint32(int(e.live) + e.used.set(p%chunkPages, hi-c*chunkPages, used)) // #nosec G115 -- at most chunkPages.
		p = hi

		// Reorder the chunk, or free it.
		if e.live == old {
			continue
		}
		s.partial.Delete(fullness{used: old, chunk: c})
		switch {
		case e.live == 0:
			s.freeChunk(c, true)
		case e.live < chunkPages:
			s.partial.Set(fullness{used: e.live, chunk: c})
		}
	}
}

// takePages takes up to want index pages, at least one, as runs: the unused
// pages of the fullest index chunks first, so sparse ones drain, then a new
// index chunk.
func (s *space) takePages(want uint64) []run {
	var out []run
	for want > 0 {
		// Take the first unused run of the fullest chunk with room.
		f, ok := s.fullest()
		if !ok {
			s.takeChunks(1, chunkIndex)
			continue
		}
		lo, hi, _ := s.table[f.chunk].used.span(1, want)
		r := run{start: f.chunk*chunkPages + lo, n: hi - lo}
		s.markPages(r, true)
		out = append(out, r)
		want -= r.n

		// Stop at a new chunk's end rather than open another.
		if _, more := s.fullest(); f.used == 0 && want != 0 && !more {
			break
		}
	}
	return out
}

// fullest returns the fullest index chunk with unused pages, passing over the
// chunk the cleaner empties.
func (s *space) fullest() (fullness, bool) {
	var out fullness
	found := false
	s.partial.Scan(func(f fullness) bool {
		out, found = f, f.chunk == 0 || f.chunk != s.evac
		return !found
	})
	return out, found
}

// takeRecord takes n consecutive index pages for a space record: an unused
// run of an index chunk, fullest first, else new index chunks.
func (s *space) takeRecord(n uint64) run {
	// Find a run in a chunk with room.
	var r run
	found := false
	if n <= chunkPages {
		s.partial.Scan(func(f fullness) bool {
			lo, hi, ok := s.table[f.chunk].used.span(n, n)
			if ok && (f.chunk == 0 || f.chunk != s.evac) {
				r, found = run{start: f.chunk*chunkPages + lo, n: hi - lo}, true
			}
			return !found
		})
	}

	// Else take new chunks.
	if !found {
		r = run{start: s.takeChunks(chunksFor(n), chunkIndex).start, n: n}
	}
	s.markPages(r, true)
	return r
}

// freeRun returns r to free space now: the pages of index chunks go unused,
// and log, value, and tail chunks under it free whole.
func (s *space) freeRun(r run) {
	for p := r.start; p < r.end(); {
		c := p / chunkPages
		hi := min(r.end(), (c+1)*chunkPages)
		switch s.table[c].kind {
		case chunkIndex:
			s.markPages(run{start: p, n: hi - p}, false)
		case chunkLog, chunkValue, chunkTail:
			s.freeChunk(c, true)
		}
		p = hi
	}
}

// hold takes the free chunks of r out of allocation for a background punch.
func (s *space) hold(r run) {
	for c := r.start / chunkPages; c < r.end()/chunkPages; c++ {
		s.free.Delete(c)
		s.table[c].kind = chunkHeld
	}
}

// unhold returns the held chunks of r to the free chunks, queuing them for
// the next punch unless punched.
func (s *space) unhold(r run, punched bool) {
	for c := r.start / chunkPages; c < r.end()/chunkPages; c++ {
		if c < uint64(len(s.table)) && s.table[c].kind == chunkHeld {
			s.freeChunk(c, !punched)
		}
	}
}

// placeValue assigns file space to a value of n bytes stored under a key of
// k bytes, after its header, at the cursor of the open chunk of stream it fits
// best. When it fits none, it opens the lowest free chunk in place of the one
// with the least room. An entry longer than a chunk takes its own run of
// chunks. It returns the offsets of the header and the
// value.
func (s *space) placeValue(k, n int, tag uint64, stream int) (entry, off uint64) {
	// Place the header and value together.
	size := uint64(valueHeaderLen(k, n) + n) // #nosec G115 -- lengths are not negative.
	switch {
	case size > chunkSize:
		// A long entry takes whole chunks of its own.
		r := s.takeChunks(chunksFor(pagesFor(int(size))), chunkValue) // #nosec G115 -- bounded by MaxValueSize.
		for c := r.start/chunkPages + 1; c < r.end()/chunkPages; c++ {
			s.table[c].kind = chunkTail
		}
		entry = r.start * pageSize
	default:
		// Else append at the cursor of the open chunk with the least room
		// that fits, opening a chunk when none fits.
		var x *extent
		for i := range s.open[stream] {
			o := &s.open[stream][i]
			if o.room() >= size && (x == nil || o.room() < x.room()) {
				x = o
			}
		}
		if x == nil {
			c := s.takeChunks(1, chunkValue).start / chunkPages
			x = s.reopen(stream, c)
		}
		entry = x.cursor
		x.cursor += size
	}
	off = entry + size - uint64(n) // #nosec G115 -- lengths are not negative.
	s.addLive(off, n, tag)
	return entry, off
}

// replayValue takes the space for a value of n bytes at off that replay found
// in commit tag, as placeValue did for the writer: a value spanning chunks in
// chunks of its own, else at the cursor of the open chunk of stream holding
// it. A value in a chunk not open opened it in place of the open chunk with
// the least room, as the replayed cursors still show.
func (s *space) replayValue(off uint64, n int, tag uint64, stream int) {
	// A long value has chunks of its own.
	first, last := off/chunkSize, (off+uint64(n)-1)/chunkSize // #nosec G115 -- lengths are not negative.
	if first != last {
		s.claim(first, chunkValue)
		for c := first + 1; c <= last; c++ {
			s.claim(c, chunkTail)
		}
		s.addLive(off, n, tag)
		return
	}

	// Else it advances its open chunk's cursor.
	x := s.openExtent(first)
	if x == nil {
		s.claim(first, chunkValue)
		x = s.reopen(stream, first)
	}
	x.cursor = max(x.cursor, off+uint64(n)) // #nosec G115 -- lengths are not negative.
	s.addLive(off, n, tag)
}

// reopen opens chunk c for stream in place of its open chunk with the least
// room, and returns it.
func (s *space) reopen(stream int, c uint64) *extent {
	x := &s.open[stream][0]
	for i := range s.open[stream] {
		if o := &s.open[stream][i]; o.room() < x.room() {
			x = o
		}
	}
	*x = extent{start: c * chunkSize, cursor: c * chunkSize, end: (c + 1) * chunkSize}
	return x
}

// closeOpen closes the open chunks of stream holding a value of frees, so a
// move never places a value in the chunk it frees one from. The writer and
// replay both call it before placing a move's values.
func (s *space) closeOpen(stream int, frees []extentRef) {
	for _, fr := range frees {
		for i := range s.open[stream] {
			if s.open[stream][i].holds(fr.off / chunkSize) {
				s.open[stream][i] = extent{}
			}
		}
	}
}

// claim makes chunk c an empty chunk of kind for a use replay found in the
// log. The writer never reuses a pending chunk, so a chunk the saved space
// holds as pending left the queues before the use, through a release the
// writer made without a record.
func (s *space) claim(c uint64, kind uint8) {
	if c < uint64(len(s.table)) && s.table[c].kind != chunkFree {
		r := chunkRun(c, 1)
		s.values = cut(s.values, r)
		s.pages = cut(s.pages, r)
	}
	s.setKind(c, kind)
}

// claimLog takes the log chunks of r for a link replay found in the log.
func (s *space) claimLog(r run) {
	for c := r.start / chunkPages; c < r.end()/chunkPages; c++ {
		s.claim(c, chunkLog)
	}
	s.log = append(s.log, r)
}

// chunkSpans calls fn with each chunk under n bytes at off and the bytes it
// holds.
func chunkSpans(off uint64, n int, fn func(c uint64, b uint32)) {
	end := off + uint64(n) // #nosec G115 -- lengths are not negative.
	for c := off / chunkSize; c*chunkSize < end; c++ {
		lo, hi := max(off, c*chunkSize), min(end, (c+1)*chunkSize)
		fn(c, uint32(hi-lo)) // #nosec G115 -- at most one chunk.
	}
}

// addLive counts n live bytes at off, placed by commit tag.
func (s *space) addLive(off uint64, n int, tag uint64) {
	chunkSpans(off, n, func(c uint64, b uint32) {
		e := &s.table[c]
		e.live += b
		e.seq = tag
	})
	s.liveBytes += int64(n)
}

// freeValue drops n live bytes at off, freed by commit tag. A chunk left
// without live bytes is pending, and closes if it is open, so an emptied
// file can shrink.
func (s *space) freeValue(off uint64, n int, tag uint64) {
	chunkSpans(off, n, func(c uint64, b uint32) {
		// Take the bytes from the chunk; an emptied chunk closes and waits.
		e := &s.table[c]
		e.live -= b
		if e.live != 0 {
			return
		}
		if x := s.openExtent(c); x != nil {
			*x = extent{}
		}
		s.pend(&s.values, pending{run: chunkRun(c, 1), tag: tag})
	})
	s.liveBytes -= int64(n)
}

// holds reports whether x is open on chunk c.
func (x extent) holds(c uint64) bool {
	return x.end != 0 && c == x.start/chunkSize
}

// room is the bytes left in x, zero when none is open.
func (x extent) room() uint64 {
	return x.end - x.cursor
}

// openExtent returns the open chunk c, or nil.
func (s *space) openExtent(c uint64) *extent {
	for i := range s.open {
		for j := range s.open[i] {
			if s.open[i][j].holds(c) {
				return &s.open[i][j]
			}
		}
	}
	return nil
}

// cut removes the pages of r from the runs in q in place, keeping tag order.
func cut(q []pending, r run) []pending {
	for i := 0; i < len(q); i++ {
		x := q[i]
		if x.end() <= r.start || x.start >= r.end() {
			continue
		}
		var parts []pending
		q, parts = cutAt(q, i, r)
		i += len(parts) - 1
	}
	return q
}

// cutAt removes the pages of r from q[i], which r overlaps, keeping tag
// order, and returns q with the parts of q[i] that remain in its place. It
// allocates only to split a run.
func cutAt(q []pending, i int, r run) ([]pending, []pending) {
	x := q[i]
	head := pending{run: run{start: x.start, n: r.start - x.start}, tag: x.tag}
	tail := pending{run: run{start: r.end(), n: x.end() - r.end()}, tag: x.tag}
	switch {
	case x.start < r.start && x.end() > r.end():
		q[i] = head
		q = slices.Insert(q, i+1, tail)
		return q, q[i : i+2]
	case x.start < r.start:
		q[i] = head
		return q, q[i : i+1]
	case x.end() > r.end():
		q[i] = tail
		return q, q[i : i+1]
	default:
		return slices.Delete(q, i, i+1), nil
	}
}

// pend queues a freed run, merging it with the previous run of the same tag.
func (s *space) pend(q *[]pending, p pending) {
	if l := len(*q); l != 0 && (*q)[l-1].tag == p.tag && (*q)[l-1].end() == p.start {
		(*q)[l-1].n += p.n
		return
	}
	*q = append(*q, p)
}

// freePages queues index pages freed by checkpoint tag.
func (s *space) freePages(pages []uint64, tag uint64) {
	slices.Sort(pages)
	for _, p := range pages {
		s.pend(&s.pages, pending{run: run{start: p, n: 1}, tag: tag})
	}
}

// due reports whether release(seq, ckpt) would free anything.
func (s *space) due(seq, ckpt uint64) bool {
	return (len(s.values) != 0 && s.values[0].tag <= seq) ||
		(len(s.pages) != 0 && s.pages[0].tag <= ckpt)
}

// release frees value chunks with tags through seq and page runs with tags
// through ckpt, and raises the marks.
func (s *space) release(seq, ckpt uint64) {
	// Free value chunks through seq.
	s.marks = pin{seq: max(s.marks.seq, seq), ckpt: max(s.marks.ckpt, ckpt)}
	i := 0
	for ; i < len(s.values) && s.values[i].tag <= seq; i++ {
		s.freeRun(s.values[i].run)
	}
	s.values = s.values[i:]

	// Free page runs through ckpt.
	i = 0
	for ; i < len(s.pages) && s.pages[i].tag <= ckpt; i++ {
		s.freeRun(s.pages[i].run)
	}
	s.pages = s.pages[i:]
}

// drain returns the chunks freed since the last drain that are still free
// below the end, merged into runs, and clears them. A freed chunk taken
// again holds data, so only the table decides what may be punched.
func (s *space) drain() []run {
	// Order the freed chunks.
	slices.SortFunc(s.released, func(a, b run) int { return cmp.Compare(a.start, b.start) })
	var out []run
	for _, r := range s.released {
		// Keep each chunk still free, joining it to the run before.
		c := r.start / chunkPages
		if c >= uint64(len(s.table)) || s.table[c].kind != chunkFree {
			continue
		}
		if l := len(out) - 1; l >= 0 && out[l].end() >= r.start {
			out[l].n = max(out[l].end(), r.end()) - out[l].start
			continue
		}
		out = append(out, r)
	}
	s.released, s.releasedPages = nil, 0
	return out
}

// usage returns the use of the file's chunks. The caller fills in the index
// pages and keys, which the checkpoint counts.
func (s *space) usage() Usage {
	// Count the file and the live values.
	u := Usage{FileBytes: pageOff(s.end()), ValueBytes: s.liveBytes}

	// Sum the chunks by kind, and the runs snapshots still hold.
	for _, e := range s.table {
		switch e.kind {
		case chunkFree, chunkHeld:
			u.FreeBytes += chunkSize
		case chunkLog:
			u.LogBytes += chunkSize
		case chunkIndex:
			u.IndexChunkBytes += chunkSize
		case chunkValue, chunkTail:
			u.ValueChunkBytes += chunkSize
		}
	}
	for _, p := range slices.Concat(s.values, s.pages) {
		u.PendingBytes += pageOff(p.n)
	}
	return u
}

// clone returns a copy of s to change and encode apart from s. The copy's
// btrees share nodes with s's until either changes them.
func (s *space) clone() *space {
	// Copy every container; the copy has released nothing yet.
	c := *s
	c.table = slices.Clone(s.table)
	c.free, c.partial = s.free.Copy(), s.partial.Copy()
	c.values, c.pages = slices.Clone(s.values), slices.Clone(s.pages)
	c.log, c.reserved = slices.Clone(s.log), slices.Clone(s.reserved)
	c.released, c.releasedPages = nil, 0
	return &c
}

// encode writes the space.
func (s *space) encode() []byte {
	// Size the buffer: a kind byte for each chunk, a page map for each index
	// chunk, and varints for the rest.
	queued := len(s.values) + len(s.pages) + len(s.reserved)
	b := make([]byte, 0, (6+2*streams)*binary.MaxVarintLen64+len(s.table)*(1+2*binary.MaxVarintLen64)+s.partial.Len()*len(pageMap{})*8+3*binary.MaxVarintLen64*(queued+len(s.log)))

	// Write the release marks and the table. A held chunk is free to any
	// space that reads it.
	u := func(v uint64) { b = binary.AppendUvarint(b, v) }
	u(s.marks.seq)
	u(s.marks.ckpt)
	u(uint64(len(s.table)))
	for _, e := range s.table {
		kind := e.kind
		if kind == chunkHeld {
			kind = chunkFree
		}
		b = append(b, kind)
		switch kind {
		case chunkIndex:
			for _, w := range e.used {
				b = binary.LittleEndian.AppendUint64(b, w)
			}
		case chunkValue, chunkTail:
			u(e.seq)
			u(uint64(e.live))
		}
	}

	// Write each stream's open chunks, as their numbers plus one, and
	// their cursors.
	for _, open := range s.open {
		for _, x := range open {
			if x.end == 0 {
				u(0)
				u(0)
				continue
			}
			u(x.start/chunkSize + 1)
			u(x.cursor - x.start)
		}
	}

	// Write the runs awaiting release and the log chunks. No saved tree
	// names a reserved run, so it is saved as a page run released at tag
	// zero.
	pages := s.pages
	if len(s.reserved) != 0 {
		pages = make([]pending, 0, len(s.reserved)+len(s.pages))
		for _, r := range s.reserved {
			pages = append(pages, pending{run: r})
		}
		pages = append(pages, s.pages...)
	}
	for _, q := range [][]pending{s.values, pages} {
		u(uint64(len(q)))
		for _, p := range q {
			u(p.tag)
			u(p.start)
			u(p.n)
		}
	}
	u(uint64(len(s.log)))
	for _, c := range s.log {
		u(c.start)
		u(c.n)
	}
	return b
}

// allocRecord takes the pages of the space's own record and returns them with
// the encoding, padded to fill them. The saved space marks its pages used, and
// taking them can lengthen the encoding past them, so it takes more pages
// until the encoding fits. Each retry asks for more pages than the last, so it
// ends. Decoding ignores the padding.
func (s *space) allocRecord() (run, []byte) {
	// Take pages until the encoding fits.
	need := pagesFor(len(s.encode()))
	r := s.takeRecord(need)
	b := s.encode()
	for pagesFor(len(b)) > need {
		s.freeRun(r)
		need = pagesFor(len(b))
		r = s.takeRecord(need)
		b = s.encode()
	}

	// Pad it to its pages.
	return r, append(b, make([]byte, pageOff(need)-int64(len(b)))...)
}

// decodeSpace reads an encoded space.
func decodeSpace(b []byte) (*space, error) {
	// Read the release marks and the table.
	s := newSpace()
	d := decoder{b: b}
	s.marks = pin{seq: d.uvarint(), ckpt: d.uvarint()}
	n := d.count()
	if n == 0 || d.err != nil {
		return nil, errors.Wrap(ErrCorrupt, "space has no chunks")
	}

	// Rebuild each chunk and the partial set from its kind.
	s.table = make([]chunk, n)
	s.partial.Clear()
	for c := range s.table {
		e := &s.table[c]
		e.kind = d.kind()
		switch e.kind {
		case chunkFree:
			s.free.Set(uint64(c)) // #nosec G115 -- slice indexes are not negative.
		case chunkLog:
		case chunkIndex:
			for i := range e.used {
				e.used[i] = d.u64()
			}
			e.live = e.used.count()
			if e.live < chunkPages {
				s.partial.Set(fullness{used: e.live, chunk: uint64(c)}) // #nosec G115 -- slice indexes are not negative.
			}
		case chunkValue, chunkTail:
			e.seq = d.uvarint()
			live := d.uvarint()
			if live > chunkSize {
				return nil, errors.Wrap(ErrCorrupt, "chunk holds more than its length")
			}
			e.live = uint32(live)
			s.liveBytes += int64(live) // #nosec G115 -- at most chunkSize.
		default:
			return nil, errors.Wrapf(ErrCorrupt, "unknown chunk kind %d", e.kind)
		}
	}
	if s.table[0].kind != chunkIndex || s.table[0].used[0]&(1<<firstPage-1) != 1<<firstPage-1 {
		return nil, errors.Wrap(ErrCorrupt, "first chunk does not hold the fixed pages")
	}

	// Read each stream's open chunks.
	for i := range s.open {
		for j := range s.open[i] {
			c, at := d.uvarint(), d.uvarint()
			if c == 0 {
				continue
			}
			if c > uint64(len(s.table)) || s.table[c-1].kind != chunkValue || at > chunkSize || s.openExtent(c-1) != nil {
				return nil, errors.Wrap(ErrCorrupt, "open chunk is not a value chunk of its own")
			}
			start := (c - 1) * chunkSize
			s.open[i][j] = extent{start: start, cursor: start + at, end: start + chunkSize}
		}
	}

	// Read the runs awaiting release and the log chunks.
	for _, q := range []*[]pending{&s.values, &s.pages} {
		for range d.count() {
			tag := d.uvarint()
			*q = append(*q, pending{tag: tag, run: run{start: d.uvarint(), n: d.uvarint()}})
		}
	}
	for range d.count() {
		s.log = append(s.log, run{start: d.uvarint(), n: d.uvarint()})
	}
	return s, d.err
}

// victim returns the chunk the cleaner should empty next and whether the
// slack is urgent, past half the live bytes, or false when the file holds too
// little slack to clean: free chunks, dead bytes in closed value chunks
// and unused pages in index chunks together at most an eighth of the live
// bytes, or less than least. While a free chunk lies below the highest value
// or index chunk, that chunk goes first, open or not, so the end can fall;
// emptying an open chunk closes it. Otherwise the chunk with the best
// cost-benefit (1-u)*age/(1+u) goes, where u is its live share and age the
// commits since a value chunk last took a value, or one for an index chunk,
// whose pages carry no date; once the dead bytes alone pass the bound. A
// victim is a value chunk with live bytes that does not start a long value,
// or an index chunk past the first while the index chunks' unused pages add
// up to a whole chunk or a chunk is free, so others can take its pages; and
// that stuck does not hold at its current stamp. seq is the latest commit.
func (s *space) victim(seq uint64, least int64, stuck map[uint64]uint64) (uint64, bool, bool) {
	// Count the index chunks' unused pages.
	var spare uint64
	s.partial.Scan(func(f fullness) bool {
		spare += chunkPages - uint64(f.used)
		return true
	})

	// Sum the slack and find the highest chunk and the best scored one.
	var dead int64
	var top, best uint64
	var bestScore float64
	haveTop, found := false, false
	for c := range s.table {
		e := &s.table[c]
		cu := uint64(c) // #nosec G115 -- slice indexes are not negative.
		var u, age float64
		switch {
		case e.kind == chunkIndex && c != 0 && (spare >= chunkPages || s.free.Len() != 0):
			u, age = float64(e.live)/chunkPages, 1
		case e.kind == chunkValue && e.live != 0 && (c+1 == len(s.table) || s.table[c+1].kind != chunkTail):
			u, age = float64(e.live)/chunkSize, float64(seq-min(seq, e.seq)+1)
		default:
			continue
		}
		if at, ok := stuck[cu]; ok && at == e.stamp() {
			continue
		}
		top, haveTop = cu, true
		if s.openExtent(cu) != nil {
			continue
		}
		dead += int64((1 - u) * chunkSize)
		score := (1 - u) * age / (1 + u)
		if !found || score > bestScore {
			best, bestScore, found = cu, score, true
		}
	}

	// Clean only past the slack bound: the highest chunk first while a
	// free chunk lies below it, else the best scored.
	slack, bound := dead+int64(s.free.Len())*chunkSize, max(s.liveBytes/8, least)
	if !haveTop || slack <= bound {
		return 0, false, false
	}
	urgent := slack > max(s.liveBytes/2, least)
	if free, ok := s.free.Min(); ok && free < top {
		return top, urgent, true
	}
	if !found || dead <= bound || bestScore == 0 {
		return 0, false, false
	}
	return best, urgent, true
}

// stamp dates what chunk e holds, so the cleaner retries a chunk it passed
// over once that changes: the last commit that placed a value in a value
// chunk, or the used pages of an index chunk.
func (e *chunk) stamp() uint64 {
	if e.kind == chunkIndex {
		return uint64(e.live)
	}
	return e.seq
}
