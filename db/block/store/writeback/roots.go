package block_store_writeback

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// HoldsRoots reports whether the inner store holds roots.
// Retiring a local root lets the collector sweep superseded blocks; a marker
// whose block was swept is dropped without upload.
func (m *MarkingStore) HoldsRoots() bool {
	return block.HoldsRoots(m.store)
}

// GetRoot reads the root from the inner store.
func (m *MarkingStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return m.store.GetRoot(ctx, name)
}

// ListRoots lists the roots of the inner store.
func (m *MarkingStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return m.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the inner store and marks the blocks it wrote.
func (m *MarkingStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Hold the write open until the observer has seen it.
	defer m.ops.end(m.ops.begin(false))

	// Mark in the storing transaction.
	if m.marker != nil {
		ctx, req := kvtx.WithBlockMarker(ctx, m.marker)
		if err := m.store.SetRoots(ctx, entries, roots); err != nil {
			return err
		}
		m.observeEntries(entries, roots)
		if !req.Applied() && slices.ContainsFunc(entries, isMarked) {
			return errUnmarkedWrite
		}
		m.invalidateTombstones(ctx, entries)
		return nil
	}

	// Mark after the write without a marker.
	if err := m.store.SetRoots(ctx, entries, roots); err != nil {
		return err
	}
	m.observeEntries(entries, roots)
	return m.markEntries(ctx, entries)
}

// PinRoot forwards a reader pin to the inner store.
func (m *MarkingStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, m.store, ref)
}

// OpenStage wraps a stage opened on the inner store, so staged writes are
// marked for upload like any other write. The stage is open in the record of
// read operations until its release, as it reads through its own view.
func (m *MarkingStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	// Record the stage as a read operation, then open it.
	n := m.ops.begin(true)
	store, release, err := block.OpenStage(ctx, m.store)
	if err != nil {
		m.ops.end(n)
		return nil, nil, err
	}
	return m.copyOver(store), m.ops.releaseRead(n, release), nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (m *MarkingStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, m.store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (m *MarkingStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, m.store, roots)
}

// RootsComplete checks the inner store's World proofs.
func (m *MarkingStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, m.store, refs)
}

// HoldCompleteRoots holds completion proofs under the inner stage. A hold
// writes no block, so there is nothing to mark.
func (m *MarkingStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, m.store, refs)
}

// _ is a type assertion
var (
	_ block.LeasedRoots = (*MarkingStore)(nil)
	_ block.RootProofs  = (*MarkingStore)(nil)
)
