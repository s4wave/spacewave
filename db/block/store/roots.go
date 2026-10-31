package block_store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (s *store) SupportsRootRetention() bool { return block.SupportsRootRetention(s.ops) }

// SetRetainedRoots forwards durable root ownership to the underlying store.
func (s *store) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRetainedRoots(ctx, s.ops, entries, roots)
}

// KeepsHoldCounts reports whether the underlying store keeps shared counts.
func (s *store) KeepsHoldCounts() bool { return block.KeepsHoldCounts(s.ops) }

// PinRoot forwards a reader pin to the underlying store.
func (s *store) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.ops, ref)
}

// OpenStage wraps a stage opened on the underlying store.
func (s *store) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	ops, release, err := block.OpenStage(ctx, s.ops)
	if err != nil {
		return nil, nil, err
	}
	return &store{ops: ops, id: s.id}, release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (s *store) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, s.ops, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (s *store) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.ops, roots)
}

// RootsComplete checks the underlying World proofs.
func (s *store) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, s.ops, refs)
}

// HoldCompleteRoots holds completion proofs under the underlying stage.
func (s *store) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, s.ops, refs)
}

// _ is a type assertion
var (
	_ block.RootRetainer       = (*store)(nil)
	_ block.CompleteRootHolder = (*store)(nil)
)
