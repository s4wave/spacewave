//go:build space_adapter_decl

package block

import (
	"context"
	"errors"
	"sync"
)

// errPendingStoreUnbuilt marks a PendingStore method declared for review.
var errPendingStoreUnbuilt = errors.New("pending store: declared, not built")

// PendingStore keeps the blocks put through it in memory and stores them only
// when a named root moves onto a block that reaches them. A writer whose
// record of a change is not its blocks, such as a SharedObject whose
// operation log rebuilds a World, writes every intermediate tree through it,
// and only the trees a root names reach the store: a page written and
// replaced between two named roots never enters the Volume's graph.
//
// Reads serve a kept block first, then the inner store. SetRoots adds to its
// entries every kept block a moved root reaches and is not yet stored, so the
// root and its tree land in one transaction, and drops them from memory once
// the inner store holds them. Sweep drops the kept blocks no live root
// reaches.
//
// Invariants:
//   - A kept block carries its complete child list, so a walk from a root
//     through kept blocks never reads the inner store to find a child.
//   - A kept block leaves memory only once the inner store holds it, held by
//     a root, or when no root Sweep is given reaches it.
//   - A kept block a moved root reaches lands in the same SetRoots as the
//     move, so the inner store never holds a root whose tree it lacks.
//   - A pin on a kept block stores the tree below it with the pin.
type PendingStore struct {
	// StoreOps is the inner store. Methods PendingStore does not override
	// pass through to it.
	StoreOps

	// mtx guards kept and keptBytes. It is never held across I/O.
	mtx sync.Mutex
	// kept holds the blocks put and not yet stored, by reference.
	kept map[string]*PutBatchEntry
	// keptBytes is the total data length of kept.
	keptBytes int
	// maxBytes bounds keptBytes. A put past it reports Full.
	maxBytes int
}

// NewPendingStore returns a PendingStore over inner that keeps at most
// maxBytes of blocks in memory.
func NewPendingStore(inner StoreOps, maxBytes int) *PendingStore {
	return &PendingStore{StoreOps: inner, kept: make(map[string]*PutBatchEntry), maxBytes: maxBytes}
}

// Full reports whether the kept blocks reached half the bound, so the writer
// names a tree before more puts must go to the inner store.
func (s *PendingStore) Full() bool {
	return false
}

// PutBlockBatch keeps each entry with a complete child list in memory and
// stores the rest. A kept entry reports that it was not stored before.
func (s *PendingStore) PutBlockBatch(context.Context, []*PutBatchEntry) ([]bool, error) {
	return nil, errPendingStoreUnbuilt
}

// GetBlock serves a kept block, or reads the inner store.
func (s *PendingStore) GetBlock(context.Context, *BlockRef) ([]byte, bool, error) {
	return nil, false, errPendingStoreUnbuilt
}

// SetRoots stores entries and every kept block a moved root reaches, and
// moves the roots, in one inner SetRoots.
func (s *PendingStore) SetRoots(context.Context, []*PutBatchEntry, []NamedRoot) error {
	return errPendingStoreUnbuilt
}

// PinRoot stores the kept tree below ref with a leased pin on it.
func (s *PendingStore) PinRoot(context.Context, *BlockRef) (func(), error) {
	return nil, errPendingStoreUnbuilt
}

// Sweep drops the kept blocks none of live reaches.
func (s *PendingStore) Sweep(live []*BlockRef) {}

// _ is a type assertion
var _ StoreOps = (*PendingStore)(nil)
