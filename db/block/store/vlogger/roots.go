package block_store_vlogger

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// SupportsRootRetention reports the underlying ownership capability.
func (s *VLoggerStore) SupportsRootRetention() bool { return block.SupportsRootRetention(s.st) }

// SetRetainedRoots forwards durable root ownership to the underlying store.
func (s *VLoggerStore) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRetainedRoots(ctx, s.st, entries, roots)
}

// KeepsHoldCounts reports whether the underlying store keeps shared counts.
func (s *VLoggerStore) KeepsHoldCounts() bool { return block.KeepsHoldCounts(s.st) }

// PinRoot forwards a reader pin to the underlying store.
func (s *VLoggerStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.st, ref)
}

// OpenStage wraps a stage opened on the underlying store.
func (s *VLoggerStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	ops, release, err := block.OpenStage(ctx, s.st)
	if err != nil {
		return nil, nil, err
	}
	return NewVLoggerStore(s.le, block_store.NewStore(s.st.GetID(), ops)), release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (s *VLoggerStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, s.st, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (s *VLoggerStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.st, roots)
}

// RootsComplete checks the underlying World proofs.
func (s *VLoggerStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, s.st, refs)
}

// HoldCompleteRoots forwards complete-root ownership to the underlying stage.
func (s *VLoggerStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, s.st, refs)
}

// _ is a type assertion
var (
	_ block.RootRetainer       = (*VLoggerStore)(nil)
	_ block.CompleteRootHolder = (*VLoggerStore)(nil)
)
