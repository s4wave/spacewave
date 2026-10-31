package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (g *GCStoreOps) SupportsRootRetention() bool { return block.SupportsRootRetention(g.store) }

// SetRetainedRoots forwards durable root ownership to the underlying store.
func (g *GCStoreOps) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRetainedRoots(ctx, g.store, entries, roots)
}

// KeepsHoldCounts reports whether the underlying store keeps shared counts.
func (g *GCStoreOps) KeepsHoldCounts() bool { return block.KeepsHoldCounts(g.store) }

// PinRoot forwards a reader pin to the underlying store.
func (g *GCStoreOps) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, g.store, ref)
}

// OpenStage forwards to the underlying store. Its writes leave this wrapper's
// graph: the stage alone owns them.
func (g *GCStoreOps) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	return block.OpenStage(ctx, g.store)
}

// ReleaseRoots forwards staging release to the underlying store.
func (g *GCStoreOps) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, g.store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (g *GCStoreOps) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, g.store, roots)
}

// RootsComplete checks the underlying World proofs.
func (g *GCStoreOps) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, g.store, refs)
}

// _ is a type assertion
var _ block.RootRetainer = (*GCStoreOps)(nil)
