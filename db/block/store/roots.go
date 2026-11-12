package block_store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// HoldsRoots reports whether the underlying store holds roots.
func (s *store) HoldsRoots() bool { return block.HoldsRoots(s.ops) }

// GetRoot reads the root from the underlying store.
func (s *store) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return s.ops.GetRoot(ctx, name)
}

// ListRoots lists the roots of the underlying store.
func (s *store) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return s.ops.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the underlying store.
func (s *store) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return s.ops.SetRoots(ctx, entries, roots)
}

// PinRoot forwards a reader pin to the underlying store. It fetches nothing.
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
	_ block.LeasedRoots = (*store)(nil)
	_ block.RootProofs  = (*store)(nil)
)
