package bucket

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// HoldsRoots reports whether the underlying store holds roots.
func (b *bucketRW) HoldsRoots() bool { return block.HoldsRoots(b.store) }

// GetRoot reads the root from the underlying store.
func (b *bucketRW) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return b.store.GetRoot(ctx, name)
}

// ListRoots lists the roots of the underlying store.
func (b *bucketRW) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return b.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the underlying store.
func (b *bucketRW) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return b.store.SetRoots(ctx, entries, roots)
}

// PinRoot forwards a reader pin to the underlying store. It fetches nothing
// itself.
func (b *bucketRW) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, b.store, ref)
}

// OpenStage wraps a stage opened on the underlying store.
func (b *bucketRW) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := block.OpenStage(ctx, b.store)
	if err != nil {
		return nil, nil, err
	}
	return &bucketRW{store: store, conf: b.conf}, release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (b *bucketRW) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, b.store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (b *bucketRW) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, b.store, roots)
}

// RootsComplete checks the underlying World proofs.
func (b *bucketRW) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, b.store, refs)
}

// _ is a type assertion
var _ block.LeasedRoots = (*bucketRW)(nil)
