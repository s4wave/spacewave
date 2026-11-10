package block_store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// CopyFill takes a copy of a block read from a lower store, such as a cache
// that queues it for its own write.
type CopyFill func(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock)

// StoreFillReads reads a lower store and passes each hit to a fill. It reads
// no other store: it serves a cache whose reads come earlier in the pipeline,
// such as a fill cache's view read before the local store, so a block is
// looked up in the cache once per read. It takes no writes.
type StoreFillReads struct {
	block.StoreOps
	fill CopyFill
}

// NewStoreFillReads reads lower, passing each hit to fill whether or not its
// refs are known: the copy must keep the refs-known state it was read with, so
// a block with unknown refs never reads back as a leaf. A Linked hit's bytes
// stay out of copies, so it is served without the fill.
func NewStoreFillReads(lower block.StoreOps, fill CopyFill) *StoreFillReads {
	return &StoreFillReads{StoreOps: lower, fill: fill}
}

// GetSupportedFeatures reports no native features: the store takes no
// writes, as a read-through over a cache's view reported.
func (s *StoreFillReads) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation scopes the lower store and fills from the scoped reads.
func (s *StoreFillReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &StoreFillReads{StoreOps: scoped, fill: s.fill}, release, nil
}

// GetBlock reads the block from lower and fills it.
func (s *StoreFillReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetStoredBlock reads the block and its refs from lower and fills it. The
// fill runs on a context without the read's cancel, as it outlives the read.
func (s *StoreFillReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := s.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	if !stored.Linked {
		s.fill(context.WithoutCancel(ctx), ref, stored)
	}
	return stored, nil
}

// PutBlock is unsupported because this store is read-only.
func (*StoreFillReads) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrReadOnly
}

// PutBlockBatch is unsupported because this store is read-only.
func (*StoreFillReads) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, ErrReadOnly
}

// RmBlock is unsupported because this store is read-only.
func (*StoreFillReads) RmBlock(context.Context, *block.BlockRef) error {
	return ErrReadOnly
}

// SetRoots is unsupported because this store is read-only.
func (*StoreFillReads) SetRoots(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error {
	return ErrReadOnly
}

// Sync reports that no durability barrier is needed for this read-only view.
func (*StoreFillReads) Sync(context.Context) (bool, error) { return true, nil }

// _ is a type assertion
var _ block.StoreOps = (*StoreFillReads)(nil)
