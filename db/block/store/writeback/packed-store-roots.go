package block_store_writeback

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// HoldsRoots reports whether the local store holds roots.
func (s *PackedStore) HoldsRoots() bool {
	return block.HoldsRoots(s.local)
}

// PinRoot forwards a reader pin to the local store.
func (s *PackedStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.local, ref)
}

// OpenStage opens a stage on the local store and wraps it, so staged writes
// go to the Log as every write does.
func (s *PackedStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	stage, release, err := block.OpenStage(ctx, s.local)
	if err != nil {
		return nil, nil, err
	}
	return &PackedStore{log: s.log, local: stage, divert: s.divert}, release, nil
}

// ReleaseRoots forwards the stage's release to the local store.
func (s *PackedStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, s.local, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (s *PackedStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.local, roots)
}

// RootsComplete checks the local store's World proofs.
func (s *PackedStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, s.local, refs)
}

// HoldCompleteRoots holds completion proofs under the local stage.
func (s *PackedStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, s.local, refs)
}

// _ is a type assertion
var (
	_ block.LeasedRoots = (*PackedStore)(nil)
	_ block.RootProofs  = (*PackedStore)(nil)
)
