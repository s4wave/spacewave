package block_store_writeback

import (
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// WriteObserver sees the blocks a Store accepts and reads back. Each call runs
// on the writer's goroutine after the write succeeded, so it must return at
// once and never fail; a write log appends a line to a buffer.
type WriteObserver interface {
	// Put sees a block the store accepted, including one it already held.
	Put(h *hash.Hash)
	// Root sees a block a named root was set to.
	Root(h *hash.Hash)
	// Fill sees a block read back from the remote to fill the local store.
	Fill(h *hash.Hash)
}

// MissObserver is a WriteObserver told when the store may accept writes it
// does not see: it was set after the store's first write, or replaced or
// detached, after which the store's writes go elsewhere. A write log uses it
// to refuse to continue across the writes it missed.
type MissObserver interface {
	WriteObserver
	// Missed reports that writes may have escaped the observer. It must
	// return at once.
	Missed()
}

// observerSlot holds the observer of a MarkingStore and every copy of it, such
// as its read operations and stages, so one SetObserver reaches them all.
type observerSlot struct {
	// p is the observer, nil for none.
	p atomic.Pointer[WriteObserver]
	// unseen is set once a write found no observer.
	unseen atomic.Bool
}

// load returns the observer for a write, nil for none, recording a write no
// observer saw.
//
// A write that finds no observer stores unseen, then loads the observer
// again; SetObserver swaps the observer, then loads unseen. The atomics are
// sequentially consistent, so for a write racing the first attach either its
// second load sees the new observer, which then sees the write, or the
// attach's load sees unseen and reports the miss: both missing would order
// the store before the second load before the swap before the attach's load,
// which then sees the store.
func (s *observerSlot) load() WriteObserver {
	// Return the observer when one is set.
	if s == nil {
		return nil
	}
	if p := s.p.Load(); p != nil {
		return *p
	}

	// Record the unseen write, then look again.
	if loadHook != nil {
		loadHook()
	}
	if !s.unseen.Load() {
		s.unseen.Store(true)
	}
	if p := s.p.Load(); p != nil {
		return *p
	}
	return nil
}

// loadHook, when set by a test, runs in load between finding no observer and
// recording the unseen write, where an attach races the write.
var loadHook func()

// SetObserver sets the observer of the store's writes, nil for none. With
// none, a write costs two atomic loads more. The observer it replaces is told
// it misses the writes from here on, and a new one is told it missed the
// writes the store accepted with none set.
func (m *MarkingStore) SetObserver(o WriteObserver) {
	// Swap the observer.
	var next *WriteObserver
	if o != nil {
		next = &o
	}
	prev := m.observer.p.Swap(next)

	// Tell each side what it misses; setting the same observer again tells
	// it nothing.
	if prev != nil && (next == nil || *prev != o) {
		if mo, ok := (*prev).(MissObserver); ok {
			mo.Missed()
		}
	}
	if mo, ok := o.(MissObserver); ok && m.observer.unseen.Load() {
		mo.Missed()
	}
}

// observeRef sees a block written alone.
func (m *MarkingStore) observeRef(ref *block.BlockRef) {
	if o := m.observer.load(); o != nil && !ref.GetEmpty() {
		o.Put(ref.GetHash())
	}
}

// observeEntries sees every stored block of a written batch and the blocks of
// its roots.
func (m *MarkingStore) observeEntries(entries []*block.PutBatchEntry, roots []block.NamedRoot) {
	o := m.observer.load()
	if o == nil {
		return
	}
	for _, entry := range entries {
		if isMarked(entry) {
			o.Put(entry.Ref.GetHash())
		}
	}
	for _, root := range roots {
		if !root.Ref.GetEmpty() {
			o.Root(root.Ref.GetHash())
		}
	}
}

// ObservePublication shows the observer the blocks and roots of an atomic
// publication before the publisher stores, dedupes or pushes any of them. A
// publication writes beside the marking store, through a block marker or a
// storage node's push. Seeing a block early only keeps its pack; seeing it
// late could let a sweep remove a pack the publication reached.
func (m *MarkingStore) ObservePublication(p *block.AtomicPublication) {
	if p == nil {
		return
	}
	m.observeEntries(p.Entries, nil)
	if o := m.observer.load(); o != nil {
		if !p.Root.GetEmpty() {
			o.Root(p.Root.GetHash())
		}
		for _, r := range p.Roots {
			if !r.Ref.GetEmpty() {
				o.Root(r.Ref.GetHash())
			}
		}
	}
}

// observeFill sees a block read back from the remote.
func (s *Store) observeFill(ref *block.BlockRef) {
	if o := s.observer.load(); o != nil && !ref.GetEmpty() {
		o.Fill(ref.GetHash())
	}
}
