package block

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
)

// ReadOnlyStore reads through a store and rejects every write with
// tx.ErrNotWrite.
//
// OpenStage opens a writable stage on the inner store: a staging scope is the
// one way to write through read-only storage, and it owns what it writes.
type ReadOnlyStore struct {
	StoreOps
}

// NewReadOnlyStore wraps store so that it rejects writes.
func NewReadOnlyStore(store StoreOps) *ReadOnlyStore {
	return &ReadOnlyStore{StoreOps: store}
}

// BeginReadOperation opens a read scope that also rejects writes.
func (s *ReadOnlyStore) BeginReadOperation(ctx context.Context) (StoreOps, func(), error) {
	store, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return NewReadOnlyStore(store), release, nil
}

// PutBlock rejects the write.
func (s *ReadOnlyStore) PutBlock(context.Context, []byte, *PutOpts) (*BlockRef, bool, error) {
	return nil, false, tx.ErrNotWrite
}

// PutBlockBatch rejects the writes.
func (s *ReadOnlyStore) PutBlockBatch(context.Context, []*PutBatchEntry) ([]bool, error) {
	return nil, tx.ErrNotWrite
}

// RmBlock rejects the delete.
func (s *ReadOnlyStore) RmBlock(context.Context, *BlockRef) error {
	return tx.ErrNotWrite
}

// Sync has nothing to fence: the store accepted no writes.
func (s *ReadOnlyStore) Sync(context.Context) (bool, error) {
	return false, nil
}

// SupportsRootRetention reports the inner store's root ownership.
func (s *ReadOnlyStore) SupportsRootRetention() bool {
	return SupportsRootRetention(s.StoreOps)
}

// KeepsHoldCounts reports whether the inner store keeps shared counts.
func (s *ReadOnlyStore) KeepsHoldCounts() bool { return KeepsHoldCounts(s.StoreOps) }

// SetRetainedRoots rejects the change of a named root.
func (s *ReadOnlyStore) SetRetainedRoots(context.Context, []*PutBatchEntry, []NamedRoot) error {
	return tx.ErrNotWrite
}

// PinRoot forwards a reader pin to the inner store.
func (s *ReadOnlyStore) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	return PinRoot(ctx, s.StoreOps, ref)
}

// OpenStage opens a writable stage on the inner store.
func (s *ReadOnlyStore) OpenStage(ctx context.Context) (StoreOps, func(), error) {
	return OpenStage(ctx, s.StoreOps)
}

// ReleaseRoots rejects the release of staging ownership.
func (s *ReadOnlyStore) ReleaseRoots(context.Context, []*BlockRef) error {
	return tx.ErrNotWrite
}

// MarkRootsComplete rejects the completion proof.
func (s *ReadOnlyStore) MarkRootsComplete(context.Context, []*BlockRef) error {
	return tx.ErrNotWrite
}

// RootsComplete checks the inner store's completion proofs.
func (s *ReadOnlyStore) RootsComplete(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return RootsComplete(ctx, s.StoreOps, refs)
}

// HoldCompleteRoots forwards stage ownership of already complete roots.
func (s *ReadOnlyStore) HoldCompleteRoots(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return HoldCompleteRoots(ctx, s.StoreOps, refs)
}

// _ is a type assertion
var (
	_ StoreOps           = (*ReadOnlyStore)(nil)
	_ RootRetainer       = (*ReadOnlyStore)(nil)
	_ CompleteRootHolder = (*ReadOnlyStore)(nil)
)
