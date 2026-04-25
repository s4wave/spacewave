package store

// span is one immutable resident byte interval in the shared span store.
//
// Spans are paged so large ranges do not require contiguous allocations and
// so overlapping logical views never need to copy data to share it. A span is
// inserted exactly once, never resized, and only removed via eviction. Spans
// are guarded by the owning engine's bcast.
type span struct {
	// off is the inclusive packfile offset of the span.
	off int64
	// size is the span length in bytes.
	size int64
	// pageSize is the in-memory page size backing pages.
	pageSize int
	// pages holds the cached bytes.
	pages [][]byte
	// pins is the number of block records retaining this span.
	pins int
	// lastUseSeq is the engine LRU sequence.
	lastUseSeq uint64
}

// newSpan builds a paged span from contiguous bytes.
func newSpan(off int64, pageSize int, data []byte) *span {
	return &span{
		off:      off,
		size:     int64(len(data)),
		pageSize: pageSize,
		pages:    buildPagedBytes(pageSize, data),
	}
}

// end returns the exclusive end offset of the span.
func (s *span) end() int64 {
	return s.off + s.size
}

// readAt copies bytes from the span into p, starting at packfile offset off.
// It returns the number of bytes written, which may be less than len(p) when
// off+len(p) extends past the end of the span.
func (s *span) readAt(p []byte, off int64) int {
	if len(p) == 0 || off < s.off || off >= s.end() {
		return 0
	}
	available := int(s.end() - off)
	if available < len(p) {
		p = p[:available]
	}
	return copyPagedBytes(p, s.pages, s.pageSize, uint64(off-s.off))
}

// copySpans copies bytes starting at off from a list of disjoint spans.
// Spans must be in ascending order with no gaps across the covered interval.
// Returns the number of bytes copied before hitting a gap, end of spans, or
// filling dst.
func copySpans(dst []byte, spans []*span, off int64) int {
	var n int
	cur := off
	for _, s := range spans {
		if len(dst[n:]) == 0 {
			return n
		}
		if cur < s.off {
			return n
		}
		if cur >= s.end() {
			continue
		}
		ncopied := s.readAt(dst[n:], cur)
		n += ncopied
		cur += int64(ncopied)
	}
	return n
}

// spansCover reports whether the given disjoint ascending spans fully cover
// [start, end) with no gaps.
func spansCover(spans []*span, start, end int64) bool {
	if end <= start {
		return true
	}
	cur := start
	for _, s := range spans {
		if cur < s.off {
			return false
		}
		if cur >= s.end() {
			continue
		}
		cur = min(end, s.end())
		if cur >= end {
			return true
		}
	}
	return false
}
