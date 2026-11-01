package block_store_writeback

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// SupportsRootRetention reports the inner store's root ownership capability.
// Retiring a local root lets the collector sweep superseded blocks; a marker
// whose block was swept is dropped without upload.
func (m *MarkingStore) SupportsRootRetention() bool {
	return block.SupportsRootRetention(m.store)
}

// KeepsHoldCounts reports whether the inner store keeps shared counts.
func (m *MarkingStore) KeepsHoldCounts() bool { return block.KeepsHoldCounts(m.store) }

// SetRetainedRoots forwards durable root ownership to the inner store and
// marks the blocks it wrote.
func (m *MarkingStore) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Mark in the storing transaction.
	if m.marker != nil {
		ctx, req := kvtx.WithBlockMarker(ctx, m.marker)
		if err := block.SetRetainedRoots(ctx, m.store, entries, roots); err != nil {
			return err
		}
		if !req.Applied() && slices.ContainsFunc(entries, isMarked) {
			return errUnmarkedWrite
		}
		m.invalidateTombstones(ctx, entries)
		return nil
	}

	if err := block.SetRetainedRoots(ctx, m.store, entries, roots); err != nil {
		return err
	}
	return m.markEntries(ctx, entries)
}

// PinRoot forwards a reader pin to the inner store.
func (m *MarkingStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, m.store, ref)
}

// OpenStage wraps a stage opened on the inner store, so staged writes are
// marked for upload like any other write.
func (m *MarkingStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := block.OpenStage(ctx, m.store)
	if err != nil {
		return nil, nil, err
	}
	return &MarkingStore{store: store, mark: m.mark, marker: m.marker}, release, nil
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
	_ block.RootRetainer       = (*MarkingStore)(nil)
	_ block.CompleteRootHolder = (*MarkingStore)(nil)
)
