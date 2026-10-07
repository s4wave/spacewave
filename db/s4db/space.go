package s4db

import (
	"encoding/binary"
	"maps"
	"slices"

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
	// tag is the commit sequence that freed a value run, or the checkpoint
	// that freed a page run.
	tag uint64
}

// extent is the value extent taking packed values, as file offsets.
type extent struct {
	// start is the first byte.
	start uint64
	// cursor is where the next value goes.
	cursor uint64
	// end is the byte after the extent, or zero when none is open.
	end uint64
}

// space tracks which pages of the file are in use. The writer owns it.
//
// Value pages are shared by packed values, so each page counts its live
// bytes and becomes free when the count reaches zero. Index pages and log
// chunks are freed whole.
type space struct {
	// free maps the first page of each free run to its length.
	free *btree.Map[uint64, uint64]
	// end is the file length in pages.
	end uint64
	// live maps each value page to its live bytes.
	live map[uint64]uint32
	// liveBytes is the total live value bytes.
	liveBytes int64
	// values holds freed value runs in tag order.
	values []pending
	// pages holds freed index pages and log chunks in tag order.
	pages []pending
	// chunks lists the log chunks in log order.
	chunks []run
	// open is the extent taking packed values.
	open extent
	// released collects runs freed since the caller last drained it, for
	// hole punching.
	released []run
	// marks are the highest release marks applied: value runs through
	// marks.seq and page runs through marks.ckpt are free.
	marks pin
	// reserved lists the runs checkpoints are writing new pages into.
	reserved []run
}

// newSpace returns the space of a new file.
func newSpace() *space {
	return &space{free: new(btree.Map[uint64, uint64]), end: firstPage, live: make(map[uint64]uint32)}
}

// alloc returns n consecutive pages from the lowest free run that holds
// them, extending the file when none does.
func (s *space) alloc(n uint64) run {
	r, ok := s.fit(n, n)
	if !ok {
		r = run{start: s.end, n: n}
		s.end += n
	}
	return r
}

// fit takes want pages from the lowest free run that holds them, or else the
// largest free run of at least least pages.
func (s *space) fit(least, want uint64) (run, bool) {
	r, ok := s.choose(least, want)
	if ok {
		s.take(r)
	}
	return r, ok
}

// choose returns the run fit would take without taking it.
func (s *space) choose(least, want uint64) (run, bool) {
	var best run
	found := false
	s.free.Scan(func(start, n uint64) bool {
		if n >= want {
			best, found = run{start: start, n: want}, true
			return false
		}
		if n >= least && n > best.n {
			best, found = run{start: start, n: n}, true
		}
		return true
	})
	return best, found
}

// lower reports whether a value of n bytes placed in a new extent would
// start below page p.
func (s *space) lower(n int, p uint64) bool {
	need := pagesFor(n)
	r, ok := s.choose(need, max(need, extentPages))
	return ok && r.start < p
}

// take removes r from the free runs, extending the file to cover it.
func (s *space) take(r run) {
	// Grow the file to cover r, freeing any gap before it.
	if r.end() > s.end {
		if r.start > s.end {
			s.addFree(run{start: s.end, n: r.start - s.end})
		}
		s.end = r.end()
	}

	// Cut r out of the free runs that overlap it.
	for _, h := range s.overlaps(r) {
		s.free.Delete(h.start)
		if h.start < r.start {
			s.free.Set(h.start, r.start-h.start)
		}
		if h.end() > r.end() {
			s.free.Set(r.end(), h.end()-r.end())
		}
	}
}

// overlaps returns the free runs that overlap r, in descending order.
func (s *space) overlaps(r run) []run {
	var hits []run
	s.free.Descend(r.end()-1, func(start, n uint64) bool {
		if start+n <= r.start {
			return false
		}
		hits = append(hits, run{start: start, n: n})
		return true
	})
	return hits
}

// addFree returns r to the free runs, merging neighbors and shrinking the
// file when r reaches its end.
func (s *space) addFree(r run) {
	// Queue r for hole punching.
	if r.n == 0 {
		return
	}
	s.released = append(s.released, r)

	// Merge with the free runs on either side.
	if prev, ok := s.before(r.start); ok && prev.end() == r.start {
		s.free.Delete(prev.start)
		r = run{start: prev.start, n: prev.n + r.n}
	}
	if n, ok := s.free.Get(r.end()); ok {
		s.free.Delete(r.end())
		r.n += n
	}

	// Shrink the file when the run reaches its end.
	if r.end() == s.end {
		s.end = r.start
		return
	}
	s.free.Set(r.start, r.n)
}

// before returns the free run starting below page.
func (s *space) before(page uint64) (run, bool) {
	// No run lies below the first page.
	if page == 0 {
		return run{}, false
	}

	// Take the first run descending from the page before.
	var r run
	ok := false
	s.free.Descend(page-1, func(start, n uint64) bool {
		r, ok = run{start: start, n: n}, true
		return false
	})
	return r, ok
}

// placeValue assigns file space to a value of n bytes from the open extent,
// opening a new extent when it does not fit.
func (s *space) placeValue(n int, tag uint64) uint64 {
	// Open a new extent at the lowest free run that fits.
	size := uint64(n) // #nosec G115 -- value lengths are not negative.
	if s.open.cursor+size > s.open.end {
		s.closeOpen(tag)
		need := pagesFor(n)
		r, ok := s.fit(need, max(need, extentPages))
		if !ok {
			r = s.alloc(max(need, extentPages))
		}
		s.open = extent{start: r.start * pageSize, cursor: r.start * pageSize, end: r.end() * pageSize}
	}

	// Place the value at the cursor.
	off := s.open.cursor
	s.open.cursor += size
	s.useValue(off, n)
	return off
}

// closeOpen frees the unwritten tail of the open extent. Its last written
// page is freed at tag when no live value remains in it.
func (s *space) closeOpen(tag uint64) {
	// Nothing is open.
	if s.open.end == 0 {
		return
	}

	// Pend the partly written last page when empty, free the unwritten
	// pages, and close.
	first := (s.open.cursor + pageSize - 1) / pageSize
	if p := s.open.cursor / pageSize; p < first && s.live[p] == 0 {
		s.pend(&s.values, pending{start: p, n: 1, tag: tag})
	}
	s.addFree(run{start: first, n: s.open.end/pageSize - first})
	s.open = extent{}
}

// spans calls fn with each page under n bytes at off and the bytes it
// holds.
func spans(off uint64, n int, fn func(page uint64, bytes uint32)) {
	end := off + uint64(n) // #nosec G115 -- value lengths are not negative.
	for p := off / pageSize; p*pageSize < end; p++ {
		lo, hi := max(off, p*pageSize), min(end, (p+1)*pageSize)
		fn(p, uint32(hi-lo)) // #nosec G115 -- at most one page.
	}
}

// useValue counts n live bytes at off. Replay calls it for values on pages
// the saved state holds as free or pending.
func (s *space) useValue(off uint64, n int) {
	spans(off, n, func(p uint64, b uint32) {
		if s.live[p] == 0 && !s.inOpen(p) {
			s.claim(run{start: p, n: 1})
		}
		s.live[p] += b
	})
	s.liveBytes += int64(n)
}

// claim takes r for a use replay found in the log. The writer never uses a
// pending page, so a used page left the queues before the use: through a
// release the writer made without a record, or, for a page of the writer's
// open extent that replay pended when its values were freed, never.
func (s *space) claim(r run) {
	s.values = cut(s.values, r)
	s.pages = cut(s.pages, r)
	s.take(r)
}

// cut removes the pages of r from the runs in q.
func cut(q []pending, r run) []pending {
	out := make([]pending, 0, len(q)+1)
	for _, x := range q {
		if x.end() <= r.start || x.start >= r.end() {
			out = append(out, x)
			continue
		}
		if x.start < r.start {
			out = append(out, pending{start: x.start, n: r.start - x.start, tag: x.tag})
		}
		if x.end() > r.end() {
			out = append(out, pending{start: r.end(), n: x.end() - r.end(), tag: x.tag})
		}
	}
	return out
}

// freeValue drops n live bytes at off, freed by commit tag.
func (s *space) freeValue(off uint64, n int, tag uint64) {
	spans(off, n, func(p uint64, b uint32) {
		s.live[p] -= b
		if s.live[p] != 0 {
			return
		}
		delete(s.live, p)
		if !s.inOpen(p) || (p+1)*pageSize <= s.open.cursor {
			s.pend(&s.values, pending{start: p, n: 1, tag: tag})
		}
	})
	s.liveBytes -= int64(n)
}

// inOpen reports whether page lies in the open extent.
func (s *space) inOpen(p uint64) bool {
	return p*pageSize >= s.open.start && p*pageSize < s.open.end
}

// pend queues a freed run, merging it with the previous run of the same tag.
func (s *space) pend(q *[]pending, p pending) {
	if l := len(*q); l != 0 {
		last := &(*q)[l-1]
		if last.tag == p.tag && last.end() == p.start {
			last.n += p.n
			return
		}
	}
	*q = append(*q, p)
}

// freePages queues index pages freed by checkpoint tag.
func (s *space) freePages(pages []uint64, tag uint64) {
	slices.Sort(pages)
	for _, p := range pages {
		s.pend(&s.pages, pending{start: p, n: 1, tag: tag})
	}
}

// due reports whether release(seq, ckpt) would free anything.
func (s *space) due(seq, ckpt uint64) bool {
	return (len(s.values) != 0 && s.values[0].tag <= seq) ||
		(len(s.pages) != 0 && s.pages[0].tag <= ckpt)
}

// release frees value runs with tags through seq and page runs with tags
// through ckpt, and raises the marks.
func (s *space) release(seq, ckpt uint64) {
	// Free value runs through seq.
	s.marks = pin{seq: max(s.marks.seq, seq), ckpt: max(s.marks.ckpt, ckpt)}
	i := 0
	for ; i < len(s.values) && s.values[i].tag <= seq; i++ {
		s.addFree(s.values[i].run)
	}
	s.values = s.values[i:]

	// Free page runs through ckpt.
	i = 0
	for ; i < len(s.pages) && s.pages[i].tag <= ckpt; i++ {
		s.addFree(s.pages[i].run)
	}
	s.pages = s.pages[i:]
}

// drain returns the parts of the runs released since the last drain that
// are still free, and clears them. A released page taken again holds data,
// so only the free map decides what may be punched.
func (s *space) drain() []run {
	var out []run
	for _, r := range s.released {
		for _, h := range s.overlaps(r) {
			start := max(h.start, r.start)
			out = append(out, run{start: start, n: min(h.end(), r.end()) - start})
		}
	}
	s.released = nil
	return out
}

// stats returns the live value bytes and the bytes value pages occupy.
func (s *space) stats() (live, held int64) {
	return s.liveBytes, int64(len(s.live)) * pageSize
}

// encode writes the space with the open extent closed.
func (s *space) encode() []byte {
	// Write the release marks, the file end, and the free runs.
	var b []byte
	u := func(v uint64) { b = binary.AppendUvarint(b, v) }
	u(s.marks.seq)
	u(s.marks.ckpt)
	u(s.end)
	u(uint64(s.free.Len())) // #nosec G115 -- lengths are not negative.
	s.free.Scan(func(start, n uint64) bool {
		u(start)
		u(n)
		return true
	})

	// Write the live value pages as runs of consecutive pages with equal
	// live bytes, each starting at its gap from the previous run. Packed
	// extents hold mostly full pages, so a filled file needs few runs.
	runs := s.liveRuns()
	u(uint64(len(runs)))
	var prev uint64
	for _, r := range runs {
		u(r.start - prev)
		u(r.n)
		u(uint64(r.bytes))
		prev = r.end()
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
	u(uint64(len(s.chunks)))
	for _, c := range s.chunks {
		u(c.start)
		u(c.n)
	}
	return b
}

// liveRun is a run of value pages that each hold bytes live bytes.
type liveRun struct {
	run
	// bytes is the live bytes of each page.
	bytes uint32
}

// liveRuns returns the live value pages as runs in page order.
func (s *space) liveRuns() []liveRun {
	var runs []liveRun
	for _, p := range slices.Sorted(maps.Keys(s.live)) {
		b := s.live[p]
		if l := len(runs); l != 0 && runs[l-1].end() == p && runs[l-1].bytes == b {
			runs[l-1].n++
			continue
		}
		runs = append(runs, liveRun{start: p, n: 1, bytes: b})
	}
	return runs
}

// decodeSpace reads an encoded space.
func decodeSpace(b []byte) (*space, error) {
	// Read the release marks, the file end, and the free runs.
	s := newSpace()
	d := decoder{b: b}
	s.marks = pin{seq: d.uvarint(), ckpt: d.uvarint()}
	s.end = d.uvarint()
	for range d.count() {
		s.free.Set(d.uvarint(), d.uvarint())
	}

	// Read the live value page runs. A run may not reach past the file end.
	var prev uint64
	for range d.count() {
		r := run{start: prev + d.uvarint(), n: d.uvarint()}
		bytes := d.uvarint()
		if r.end() > s.end || r.end() < r.start || bytes > pageSize {
			return nil, ErrCorrupt
		}
		for p := r.start; p < r.end(); p++ {
			s.live[p] = uint32(bytes)
		}
		s.liveBytes += pageOff(r.n) / pageSize * int64(bytes)
		prev = r.end()
	}

	// Read the runs awaiting release and the log chunks.
	for _, q := range []*[]pending{&s.values, &s.pages} {
		for range d.count() {
			tag := d.uvarint()
			*q = append(*q, pending{tag: tag, start: d.uvarint(), n: d.uvarint()})
		}
	}
	for range d.count() {
		s.chunks = append(s.chunks, run{start: d.uvarint(), n: d.uvarint()})
	}
	return s, d.err
}
