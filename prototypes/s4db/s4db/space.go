package s4db

import (
	"encoding/binary"
	"maps"
	"slices"

	"github.com/tidwall/btree"
)

// extentPages is the size of a value extent or log chunk when free space
// allows: 1 MiB, large enough that packed writes reach full device speed.
const extentPages = 256

// run is a range of pages.
type run struct {
	// start is the first page.
	start uint64
	// n is the page count.
	n uint64
}

// pending is a run freed at tag that stays allocated until no snapshot can
// still read it.
type pending struct {
	run
	// tag is the commit sequence that freed a value run, or the checkpoint
	// that freed a page run.
	tag uint64
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
	// open is the value extent taking packed values, as file offsets.
	open struct{ start, cursor, end uint64 }
	// released collects runs freed since the caller last drained it, for
	// hole punching.
	released []run
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
		s.unrelease(r)
	}
	return r
}

// fit takes want pages from the lowest free run that holds them, or else the
// largest free run of at least least pages.
func (s *space) fit(least, want uint64) (run, bool) {
	// Scan the free runs from the lowest page.
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

	// Take the chosen pages.
	if found {
		s.take(best)
	}
	return best, found
}

// take removes r from the free runs, extending the file to cover it.
func (s *space) take(r run) {
	// Grow the file to cover r, freeing any gap before it.
	if r.start+r.n > s.end {
		if r.start > s.end {
			s.addFree(run{start: s.end, n: r.start - s.end})
		}
		s.end = r.start + r.n
	}

	// Find the free runs that overlap r.
	var hits []run
	s.free.Descend(r.start+r.n-1, func(start, n uint64) bool {
		if start+n <= r.start {
			return false
		}
		hits = append(hits, run{start: start, n: n})
		return true
	})

	// Cut r out of them and out of the runs awaiting punching.
	s.unrelease(r)
	for _, h := range hits {
		s.free.Delete(h.start)
		if h.start < r.start {
			s.free.Set(h.start, r.start-h.start)
		}
		if h.start+h.n > r.start+r.n {
			s.free.Set(r.start+r.n, h.start+h.n-r.start-r.n)
		}
	}
}

// unrelease drops r from the runs awaiting hole punching, since it is about
// to hold data.
func (s *space) unrelease(r run) {
	out := s.released[:0]
	for _, x := range s.released {
		if x.start+x.n <= r.start || x.start >= r.start+r.n {
			out = append(out, x)
			continue
		}
		if x.start < r.start {
			out = append(out, run{start: x.start, n: r.start - x.start})
		}
		if x.start+x.n > r.start+r.n {
			out = append(out, run{start: r.start + r.n, n: x.start + x.n - r.start - r.n})
		}
	}
	s.released = out
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
	if prev, n, ok := s.before(r.start); ok && prev+n == r.start {
		s.free.Delete(prev)
		r = run{start: prev, n: n + r.n}
	}
	if n, ok := s.free.Get(r.start + r.n); ok {
		s.free.Delete(r.start + r.n)
		r.n += n
	}

	// Shrink the file when the run reaches its end.
	if r.start+r.n == s.end {
		s.end = r.start
		return
	}
	s.free.Set(r.start, r.n)
}

// before returns the free run starting below page.
func (s *space) before(page uint64) (uint64, uint64, bool) {
	// No run lies below the first page.
	if page == 0 {
		return 0, 0, false
	}

	// Take the first run descending from the page before.
	var start, n uint64
	ok := false
	s.free.Descend(page-1, func(k, v uint64) bool {
		start, n, ok = k, v, true
		return false
	})
	return start, n, ok
}

// placeValue assigns file space to a value of n bytes from the open extent,
// opening a new extent when it does not fit.
func (s *space) placeValue(n int, tag uint64) uint64 {
	// Open a new extent at the lowest free run that fits.
	if s.open.cursor+uint64(n) > s.open.end {
		s.closeOpen(tag)
		need := (uint64(n) + pageSize - 1) / pageSize
		r, ok := s.fit(need, max(need, extentPages))
		if !ok {
			r = s.alloc(max(need, extentPages))
		}
		s.open.start, s.open.cursor, s.open.end = r.start*pageSize, r.start*pageSize, (r.start+r.n)*pageSize
	}

	// Place the value at the cursor.
	off := s.open.cursor
	s.open.cursor += uint64(n)
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
		s.pend(&s.values, pending{run: run{start: p, n: 1}, tag: tag})
	}
	s.addFree(run{start: first, n: s.open.end/pageSize - first})
	s.open.start, s.open.cursor, s.open.end = 0, 0, 0
}

// spans calls fn with each page under n bytes at off and the bytes it
// holds.
func spans(off uint64, n int, fn func(page uint64, bytes uint32)) {
	end := off + uint64(n)
	for p := off / pageSize; p*pageSize < end; p++ {
		lo, hi := max(off, p*pageSize), min(end, (p+1)*pageSize)
		fn(p, uint32(hi-lo))
	}
}

// useValue counts n live bytes at off. Recovery calls it for values the
// saved state recorded as free.
func (s *space) useValue(off uint64, n int) {
	spans(off, n, func(p uint64, b uint32) {
		if s.live[p] == 0 && !s.inOpen(p) {
			s.take(run{start: p, n: 1})
		}
		s.live[p] += b
	})
	s.liveBytes += int64(n)
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
			s.pend(&s.values, pending{run: run{start: p, n: 1}, tag: tag})
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
		if last.tag == p.tag && last.start+last.n == p.start {
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
		s.pend(&s.pages, pending{run: run{start: p, n: 1}, tag: tag})
	}
}

// release frees value runs with tags through seq and page runs with tags
// through ckpt.
func (s *space) release(seq, ckpt uint64) {
	// Free value runs through seq.
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

// drain returns and clears the released runs.
func (s *space) drain() []run {
	r := s.released
	s.released = nil
	return r
}

// stats returns the live value bytes and the bytes value pages occupy.
func (s *space) stats() (live, held int64) {
	return s.liveBytes, int64(len(s.live)) * pageSize
}

// freeBytes returns the bytes in free runs below the file end.
func (s *space) freeBytes() int64 {
	var n uint64
	s.free.Scan(func(_, run uint64) bool {
		n += run
		return true
	})
	return int64(n) * pageSize
}

// encode writes the space with the open extent closed.
func (s *space) encode() []byte {
	// Write the file end and the free runs.
	var b []byte
	u := func(v uint64) { b = binary.AppendUvarint(b, v) }
	u(s.end)
	u(uint64(s.free.Len()))
	s.free.Scan(func(start, n uint64) bool {
		u(start)
		u(n)
		return true
	})

	// Write the live bytes of each value page.
	u(uint64(len(s.live)))
	for _, p := range slices.Sorted(maps.Keys(s.live)) {
		u(p)
		u(uint64(s.live[p]))
	}

	// Write the runs awaiting release and the log chunks.
	for _, q := range [][]pending{s.values, s.pages} {
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

// decodeSpace reads an encoded space.
func decodeSpace(b []byte) (*space, error) {
	// Read the file end and the free runs.
	s := newSpace()
	d := decoder{b: b}
	s.end = d.uvarint()
	for range d.uvarint() {
		s.free.Set(d.uvarint(), d.uvarint())
	}

	// Read the live bytes of each value page.
	for range d.uvarint() {
		p := d.uvarint()
		s.live[p] = uint32(d.uvarint())
		s.liveBytes += int64(s.live[p])
	}

	// Read the runs awaiting release and the log chunks.
	for _, q := range []*[]pending{&s.values, &s.pages} {
		for range d.uvarint() {
			tag := d.uvarint()
			*q = append(*q, pending{tag: tag, run: run{start: d.uvarint(), n: d.uvarint()}})
		}
	}
	for range d.uvarint() {
		s.chunks = append(s.chunks, run{start: d.uvarint(), n: d.uvarint()})
	}
	return s, d.err
}
