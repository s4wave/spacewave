package block_store_s3

import (
	"cmp"
	"container/list"
	"sync"

	"github.com/tidwall/btree"
)

// spanOverhead is the bytes the buffer charges each span beyond its data, for
// its record, tree item and list element, so a buffer of many small spans
// stays within its limit too.
const spanOverhead = 128

// spanBuffer holds the bytes of the reach reads a subtree walk made, so the
// walk's next reads of the blocks inside them cut those blocks from memory
// rather than read them again.
//
// Invariants: a span holds bytes read from its pack at its offset. A pack's
// bytes never change once written, and every block cut from a span is checked
// against its hash, so a span outliving its pack answers nothing wrong. The
// buffer needs no invalidation. No held span lies inside another of its pack,
// so the spans of a pack, ordered by offset, end in the same order, and the
// one starting nearest at or before an offset is the only one that can hold a
// read from there. The spans and their overhead stay within limit bytes, the
// oldest dropped first.
type spanBuffer struct {
	// limit bounds the bytes the spans are charged.
	limit int
	// mtx guards the fields below.
	mtx sync.Mutex
	// spans holds every span by pack and offset.
	spans *btree.BTreeG[*heldSpan]
	// order holds every span, oldest first.
	order list.List
	// held is the bytes the spans are charged.
	held int
}

// heldSpan is one span of a pack's bytes.
type heldSpan struct {
	// id is the pack.
	id string
	// off is the offset of data in the pack.
	off int64
	// data is the span's bytes.
	data []byte
	// at is the span's element in order.
	at *list.Element
}

// end returns the offset just past the span.
func (sp *heldSpan) end() int64 {
	return sp.off + int64(len(sp.data))
}

// cost returns the bytes the span is charged.
func (sp *heldSpan) cost() int {
	return len(sp.data) + spanOverhead
}

// spanLess orders spans by pack and offset.
func spanLess(a, b *heldSpan) bool {
	return cmp.Or(cmp.Compare(a.id, b.id), cmp.Compare(a.off, b.off)) < 0
}

// newSpanBuffer returns an empty buffer of at most limit bytes.
func newSpanBuffer(limit int) *spanBuffer {
	return &spanBuffer{limit: limit, spans: btree.NewBTreeGOptions(spanLess, btree.Options{NoLocks: true})}
}

// add holds data, the bytes of pack id from off, dropping the spans it covers
// and then the oldest spans to stay within the limit. A span another span
// covers, or one over the limit, is not held.
func (b *spanBuffer) add(id string, off int64, data []byte) {
	// Hold nothing for an empty span or one the buffer cannot fit.
	sp := &heldSpan{id: id, off: off, data: data}
	if len(data) == 0 || sp.cost() > b.limit {
		return
	}
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Skip a span the nearest one starting at or before it already covers.
	if prev := b.nearestLocked(id, off); prev != nil && prev.end() >= sp.end() {
		return
	}

	// Drop the spans starting inside sp that sp covers. Since ends follow
	// starts, the covered spans run from off up to the first ending past
	// sp.
	var covered []*heldSpan
	b.spans.Ascend(sp, func(next *heldSpan) bool {
		if next.id != id || next.end() > sp.end() {
			return false
		}
		covered = append(covered, next)
		return true
	})
	for _, next := range covered {
		b.dropLocked(next)
	}

	// Drop the oldest spans until it fits, then hold it.
	for b.held+sp.cost() > b.limit && b.order.Len() != 0 {
		b.dropLocked(b.order.Front().Value.(*heldSpan))
	}
	b.spans.Set(sp)
	sp.at = b.order.PushBack(sp)
	b.held += sp.cost()
}

// dropLocked removes sp. The caller holds mtx.
func (b *spanBuffer) dropLocked(sp *heldSpan) {
	b.spans.Delete(sp)
	b.order.Remove(sp.at)
	b.held -= sp.cost()
}

// read returns the bytes of pack id from off to off+size when a span holds
// all of them, sharing the span's memory, or false.
func (b *spanBuffer) read(id string, off int64, size int) ([]byte, bool) {
	// Only the span starting nearest at or before off can hold the read.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	sp := b.nearestLocked(id, off)
	if sp == nil || off+int64(size) > sp.end() {
		return nil, false
	}
	start := off - sp.off
	return sp.data[start : start+int64(size)], true
}

// nearestLocked returns the span of pack id starting nearest at or before off,
// or nil. The caller holds mtx.
func (b *spanBuffer) nearestLocked(id string, off int64) *heldSpan {
	var found *heldSpan
	b.spans.Descend(&heldSpan{id: id, off: off}, func(sp *heldSpan) bool {
		if sp.id == id {
			found = sp
		}
		return false
	})
	return found
}
