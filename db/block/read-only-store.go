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
func (s *ReadOnlyStore) PutBlockBatch(context.Context, []*PutBatchEntry) error {
	return tx.ErrNotWrite
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

// SetRetainedRoot rejects the change of a named root.
func (s *ReadOnlyStore) SetRetainedRoot(context.Context, string, *BlockRef) error {
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

// RootComplete checks the inner store's completion proof.
func (s *ReadOnlyStore) RootComplete(ctx context.Context, ref *BlockRef) (bool, error) {
	return RootComplete(ctx, s.StoreOps, ref)
}

// _ is a type assertion
var (
	_ StoreOps     = (*ReadOnlyStore)(nil)
	_ RootRetainer = (*ReadOnlyStore)(nil)
)
