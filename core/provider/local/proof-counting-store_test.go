package provider_local

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// proofCountingStore counts the completion proof reads made through a store.
// Each read is one volume transaction.
type proofCountingStore struct {
	block_store.Store
	// reads counts the proof read calls.
	reads atomic.Int32
}

// SupportsRootRetention reports the underlying ownership capability.
func (s *proofCountingStore) SupportsRootRetention() bool {
	return block.SupportsRootRetention(s.Store)
}

// KeepsHoldCounts reports whether the underlying store keeps shared counts.
func (s *proofCountingStore) KeepsHoldCounts() bool {
	return block.KeepsHoldCounts(s.Store)
}

// SetRetainedRoots forwards durable root ownership to the underlying store.
func (s *proofCountingStore) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRetainedRoots(ctx, s.Store, entries, roots)
}

// PinRoot forwards a reader pin to the underlying store.
func (s *proofCountingStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.Store, ref)
}

// OpenStage forwards the stage to the underlying store.
func (s *proofCountingStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	return block.OpenStage(ctx, s.Store)
}

// ReleaseRoots forwards staging release to the underlying store.
func (s *proofCountingStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, s.Store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (s *proofCountingStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.Store, roots)
}

// RootsComplete counts and forwards a proof read.
func (s *proofCountingStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.reads.Add(1)
	return block.RootsComplete(ctx, s.Store, refs)
}

// _ is a type assertion
var _ block.RootRetainer = (*proofCountingStore)(nil)
