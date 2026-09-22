package bucket

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (b *bucketRW) SupportsRootRetention() bool { return block.SupportsRootRetention(b.store) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (b *bucketRW) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, b.store, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (b *bucketRW) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, b.store, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (b *bucketRW) MarkRootsComplete(ctx context.Context, proofs []block.RootProof) error {
	return block.MarkRootsComplete(ctx, b.store, proofs)
}

// RootComplete checks the underlying World proof.
func (b *bucketRW) RootComplete(ctx context.Context, ref *block.BlockRef, domain ...string) (bool, error) {
	return block.RootComplete(ctx, b.store, ref, domain...)
}
