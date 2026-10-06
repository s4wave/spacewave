package bucket

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (b *bucketRW) SupportsRootRetention() bool { return block.SupportsRootRetention(b.store) }

// SetRetainedRoots forwards durable root ownership to the underlying store.
func (b *bucketRW) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRetainedRoots(ctx, b.store, entries, roots)
}

// PinRoot forwards a reader pin to the underlying store.
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

// RootComplete checks the underlying World proof.
func (b *bucketRW) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, b.store, ref)
}

// _ is a type assertion
var _ block.RootRetainer = (*bucketRW)(nil)
