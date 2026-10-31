package block

import "context"

// SupportsRootRetention reports the underlying ownership capability.
func (b *StoreRW) SupportsRootRetention() bool { return SupportsRootRetention(b.writeHandle) }

// SetRetainedRoots forwards durable root ownership to the underlying store.
func (b *StoreRW) SetRetainedRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error {
	return SetRetainedRoots(ctx, b.writeHandle, entries, roots)
}

// KeepsHoldCounts reports whether the underlying store keeps shared counts.
func (b *StoreRW) KeepsHoldCounts() bool { return KeepsHoldCounts(b.writeHandle) }

// PinRoot forwards a reader pin to the underlying store.
func (b *StoreRW) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	return PinRoot(ctx, b.writeHandle, ref)
}

// OpenStage opens a stage on the write handle and keeps reads on the read
// handle.
func (b *StoreRW) OpenStage(ctx context.Context) (StoreOps, func(), error) {
	// Open the stage on the write handle.
	writeHandle, release, err := OpenStage(ctx, b.writeHandle)
	if err != nil {
		return nil, nil, err
	}

	// Copy the store with its writes routed to the stage.
	staged := *b
	staged.writeHandle = writeHandle
	return &staged, release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (b *StoreRW) ReleaseRoots(ctx context.Context, refs []*BlockRef) error {
	return ReleaseRoots(ctx, b.writeHandle, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (b *StoreRW) MarkRootsComplete(ctx context.Context, roots []*BlockRef) error {
	return MarkRootsComplete(ctx, b.writeHandle, roots)
}

// RootsComplete checks the underlying World proofs.
func (b *StoreRW) RootsComplete(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return RootsComplete(ctx, b.writeHandle, refs)
}

// _ is a type assertion
var _ RootRetainer = (*StoreRW)(nil)
