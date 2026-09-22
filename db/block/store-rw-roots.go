package block

import "context"

// SupportsRootRetention reports the underlying ownership capability.
func (b *StoreRW) SupportsRootRetention() bool { return SupportsRootRetention(b.writeHandle) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (b *StoreRW) SetRetainedRoot(ctx context.Context, name string, ref *BlockRef) error {
	return SetRetainedRoot(ctx, b.writeHandle, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (b *StoreRW) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	return PinRoot(ctx, b.writeHandle, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (b *StoreRW) MarkRootsComplete(ctx context.Context, proofs []RootProof) error {
	return MarkRootsComplete(ctx, b.writeHandle, proofs)
}

// RootComplete checks the underlying World proof.
func (b *StoreRW) RootComplete(ctx context.Context, ref *BlockRef, domain ...string) (bool, error) {
	return RootComplete(ctx, b.writeHandle, ref, domain...)
}
