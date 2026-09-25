package block_store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (s *store) SupportsRootRetention() bool { return block.SupportsRootRetention(s.ops) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (s *store) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, s.ops, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (s *store) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.ops, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (s *store) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.ops, roots)
}

// RootComplete checks the underlying World proof.
func (s *store) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, s.ops, ref)
}
