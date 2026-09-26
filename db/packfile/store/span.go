package store

import "container/list"

// span is one immutable resident byte interval in the shared span store.
//
// A span keeps the transport response slice as fetched, so admitting bytes
// never copies them. A span is inserted exactly once, never resized, and only
// removed by eviction or failed verification. Spans are guarded by the owning
// engine's bcast.
type span struct {
	// off is the inclusive packfile offset of the span.
	off int64
	// size is the span length in bytes.
	size int64
	// data holds the resident bytes.
	data []byte
	// pins is the number of block records retaining this span.
	pins int
	// lastUseSeq is the budget-wide LRU sequence of the last use.
	lastUseSeq uint64
	// lru is the element in the engine's unpinned LRU list, nil while pinned.
	lru *list.Element
}

// newSpan builds a span that takes ownership of data.
func newSpan(off int64, data []byte) *span {
	return &span{off: off, size: int64(len(data)), data: data}
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
	return copy(p, s.data[off-s.off:])
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
