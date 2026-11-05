package block

import "context"

// SupportsRootRetention reports the underlying ownership capability.
func (b *StoreRW) SupportsRootRetention() bool { return SupportsRootRetention(b.writeHandle) }

// GetRoot reads the root from the write handle, the store that moves it, so a
// Previous read here is the reference SetRoots compares against.
func (b *StoreRW) GetRoot(ctx context.Context, name string) (*BlockRef, error) {
	return b.writeHandle.GetRoot(ctx, name)
}

// ListRoots lists the roots of the write handle.
func (b *StoreRW) ListRoots(ctx context.Context, prefix string, fn func(NamedRoot) error) error {
	return b.writeHandle.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the write handle.
func (b *StoreRW) SetRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error {
	return b.writeHandle.SetRoots(ctx, entries, roots)
}

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
