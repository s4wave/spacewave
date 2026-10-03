package block_store_writeback

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the inner store's root ownership capability.
// Retiring a local root lets the collector sweep superseded blocks; a marker
// whose block was swept is dropped without upload.
func (m *MarkingStore) SupportsRootRetention() bool {
	return block.SupportsRootRetention(m.store)
}

// SetRetainedRoot forwards durable root ownership to the inner store.
func (m *MarkingStore) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, m.store, name, ref)
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
	return NewMarkingStore(store, m.mark), release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (m *MarkingStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, m.store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (m *MarkingStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, m.store, roots)
}

// RootComplete checks the inner store's World proof.
func (m *MarkingStore) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, m.store, ref)
}

// _ is a type assertion
var _ block.RootRetainer = (*MarkingStore)(nil)
