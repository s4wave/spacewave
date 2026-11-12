package block

import "context"

// HoldsRoots reports whether the underlying store holds roots.
func (b *StoreRW) HoldsRoots() bool { return HoldsRoots(b.StoreOps) }

// PinRoot forwards a reader pin to the underlying store. It fetches nothing
// itself.
func (b *StoreRW) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	return PinRoot(ctx, b.StoreOps, ref)
}

// OpenStage opens a stage on the write handle and keeps reads on the read
// handle.
func (b *StoreRW) OpenStage(ctx context.Context) (StoreOps, func(), error) {
	// Open the stage on the write handle.
	writeHandle, release, err := OpenStage(ctx, b.StoreOps)
	if err != nil {
		return nil, nil, err
	}

	// Copy the store with its writes routed to the stage.
	staged := *b
	staged.StoreOps = writeHandle
	return &staged, release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (b *StoreRW) ReleaseRoots(ctx context.Context, refs []*BlockRef) error {
	return ReleaseRoots(ctx, b.StoreOps, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (b *StoreRW) MarkRootsComplete(ctx context.Context, roots []*BlockRef) error {
	return MarkRootsComplete(ctx, b.StoreOps, roots)
}

// RootsComplete checks the underlying World proofs.
func (b *StoreRW) RootsComplete(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return RootsComplete(ctx, b.StoreOps, refs)
}

// _ is a type assertion
var _ LeasedRoots = (*StoreRW)(nil)
