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

// RootComplete checks the underlying World proof.
func (s *VLoggerStore) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, s.st, ref)
}

// _ is a type assertion
var _ block.RootRetainer = (*VLoggerStore)(nil)
