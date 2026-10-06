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

// RootComplete checks the underlying World proof.
func (g *GCStoreOps) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, g.store, ref)
}

// _ is a type assertion
var _ block.RootRetainer = (*GCStoreOps)(nil)
