package block_store_vlogger

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// SupportsRootRetention reports the underlying ownership capability.
func (s *VLoggerStore) SupportsRootRetention() bool { return block.SupportsRootRetention(s.st) }

// GetRoot reads the root from the underlying store.
func (s *VLoggerStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return s.st.GetRoot(ctx, name)
}

// ListRoots lists the roots of the underlying store.
func (s *VLoggerStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return s.st.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the underlying store.
func (s *VLoggerStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return s.st.SetRoots(ctx, entries, roots)
}

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
