package block_store_s3

import (
	"slices"
	"sort"
	"sync"
)

// spanBuffer holds the bytes of the reach reads a subtree walk made, so the
// walk's next reads of the blocks inside them cut those blocks from memory
// rather than read them again.
//
// Invariant: a span holds bytes read from its pack at its offset. A pack's
// bytes never change once written, and every block cut from a span is checked
// against its hash, so a span outliving its pack or naming a reused pack
// number answers nothing wrong; the buffer needs no invalidation. It holds at
// most limit bytes, dropping the oldest span first.
type spanBuffer struct {
	// limit bounds the bytes of the spans held.
	limit int
	// mtx guards the fields below.
	mtx sync.Mutex
	// packs holds each pack's spans by offset.
	packs map[string][]*heldSpan
	// order holds every span, oldest first.
	order []*heldSpan
	// held is the bytes of the spans held.
	held int
	// longest is the length of the longest span added, so a read looks back
	// only at spans that start close enough to reach it.
	longest int
}

// heldSpan is one span of a pack's bytes.
type heldSpan struct {
	// id is the pack.
	id string
	// off is the offset of data in the pack.
	off int64
	// data is the span's bytes.
	data []byte
}

// newSpanBuffer returns an empty buffer of at most limit bytes.
func newSpanBuffer(limit int) *spanBuffer {
	return &spanBuffer{limit: limit, packs: make(map[string][]*heldSpan)}
}

// add holds data, the bytes of pack id from off, dropping the oldest spans to
// stay within the limit. A span over the limit is not held.
func (b *spanBuffer) add(id string, off int64, data []byte) {
	// Hold nothing for an empty span or one the buffer cannot fit.
	if len(data) == 0 || len(data) > b.limit {
		return
	}
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Drop the oldest spans until data fits.
	for b.held+len(data) > b.limit && len(b.order) != 0 {
		b.dropLocked(b.order[0])
		b.order = b.order[1:]
	}

	// Insert the span after the spans starting at or before it.
	sp := &heldSpan{id: id, off: off, data: data}
	spans := b.packs[id]
	b.packs[id] = slices.Insert(spans, spansAfter(spans, off), sp)
	b.order = append(b.order, sp)
	b.held += len(data)
	b.longest = max(b.longest, len(data))
}

// dropLocked removes sp from its pack's spans. The caller removes it from
// order and holds mtx.
func (b *spanBuffer) dropLocked(sp *heldSpan) {
	spans := b.packs[sp.id]
	if i := slices.Index(spans, sp); i >= 0 {
		spans = slices.Delete(spans, i, i+1)
	}
	if len(spans) == 0 {
		delete(b.packs, sp.id)
	} else {
		b.packs[sp.id] = spans
	}
	b.held -= len(sp.data)
}

// read returns the bytes of pack id from off to off+size when one span holds
// all of them, sharing the span's memory, or false.
func (b *spanBuffer) read(id string, off int64, size int) ([]byte, bool) {
	// Hold the buffer for the lookup.
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Look at the spans starting at or before off, nearest first, back to
	// the furthest a span could start and still reach off; spans of one pack
	// may overlap, so an earlier one can reach further.
	spans := b.packs[id]
	for i := spansAfter(spans, off) - 1; i >= 0; i-- {
		sp := spans[i]
		start := off - sp.off
		if start >= int64(b.longest) {
			break
		}
		if start+int64(size) <= int64(len(sp.data)) {
			return sp.data[start : start+int64(size)], true
		}
	}
	return nil, false
}

// spansAfter returns the index of the first of spans, sorted by offset, that
// starts after off.
func spansAfter(spans []*heldSpan, off int64) int {
	return sort.Search(len(spans), func(i int) bool { return spans[i].off > off })
}
