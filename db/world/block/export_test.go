package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// ReadThrough wraps the engine's block store: get reads every block through
// the store it wraps, and put sees every block written to it.
type ReadThrough struct {
	// Get reads ref from store.
	Get func(ctx context.Context, store block.StoreOps, ref *block.BlockRef) ([]byte, bool, error)
	// Put sees ref after store writes it.
	Put func(ref *block.BlockRef)
}

// WrapBlockStore routes the engine's block reads and writes through rt.
func WrapBlockStore(e *Engine, rt ReadThrough) {
	// Wrap the store under the publication lock, keeping its pins, stages and
	// proofs.
	locked := e.bcast.Lock()
	defer locked.Unlock()
	e.writeBlockStore = newReadThroughStore(e.writeBlockStore, rt)
}

// newReadThroughStore wraps store's reads and writes in rt.
func newReadThroughStore(store block.StoreOps, rt ReadThrough) *readThroughStore {
	leases, _ := store.(block.LeasedRoots)
	proofs, _ := store.(block.RootProofs)
	return &readThroughStore{StoreOps: store, LeasedRoots: leases, RootProofs: proofs, rt: rt}
}

// readThroughStore is a block store whose reads and writes pass through a
// ReadThrough.
type readThroughStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
	// rt sees each read and write.
	rt ReadThrough
}

// GetBlock reads ref through rt.
func (s *readThroughStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.rt.Get(ctx, s.StoreOps, ref)
}

// PutBlock writes data and shows its ref to rt.
func (s *readThroughStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err == nil && s.rt.Put != nil {
		s.rt.Put(ref)
	}
	return ref, existed, err
}

// PutBlockBatch writes entries and shows their refs to rt.
func (s *readThroughStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed, err := s.StoreOps.PutBlockBatch(ctx, entries)
	if err == nil && s.rt.Put != nil {
		for _, ent := range entries {
			s.rt.Put(ent.Ref)
		}
	}
	return existed, err
}

// OpenStage opens a stage on the wrapped store whose reads and writes also
// pass through rt.
func (s *readThroughStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	staged, release, err := s.LeasedRoots.OpenStage(ctx)
	if err != nil {
		return nil, nil, err
	}
	return newReadThroughStore(staged, s.rt), release, nil
}
