package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (g *GCStoreOps) SupportsRootRetention() bool { return block.SupportsRootRetention(g.store) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (g *GCStoreOps) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, g.store, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (g *GCStoreOps) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, g.store, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (g *GCStoreOps) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, g.store, roots)
}

// RootComplete checks the underlying World proof.
func (g *GCStoreOps) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, g.store, ref)
}
