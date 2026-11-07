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

// observerSlot holds the observer of a MarkingStore and every copy of it, such
// as its read operations and stages, so one SetObserver reaches them all.
type observerSlot struct {
	// p is the observer, nil for none.
	p atomic.Pointer[WriteObserver]
}

// load returns the observer, nil for none.
func (s *observerSlot) load() WriteObserver {
	if s == nil {
		return nil
	}
	if p := s.p.Load(); p != nil {
		return *p
	}
	return nil
}

// SetObserver sets the observer of the store's writes, nil for none. With
// none, a write costs one atomic load more.
func (m *MarkingStore) SetObserver(o WriteObserver) {
	if o == nil {
		m.observer.p.Store(nil)
		return
	}
	m.observer.p.Store(&o)
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

// observeFill sees a block read back from the remote.
func (s *Store) observeFill(ref *block.BlockRef) {
	if o := s.observer.load(); o != nil && !ref.GetEmpty() {
		o.Fill(ref.GetHash())
	}
}
