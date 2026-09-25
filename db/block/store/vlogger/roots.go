package block_store_vlogger

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (s *VLoggerStore) SupportsRootRetention() bool { return block.SupportsRootRetention(s.st) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (s *VLoggerStore) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, s.st, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (s *VLoggerStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.st, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (s *VLoggerStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.st, roots)
}

// RootComplete checks the underlying World proof.
func (s *VLoggerStore) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, s.st, ref)
}
