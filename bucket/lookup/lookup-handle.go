package bucket_lookup

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
)

var (
	// ErrNotImplemented is returned for operations not implemented by Lookup().
	ErrNotImplemented = errors.New("operation not implemented by lookup controller")
)

// lookupBucket implements bucket.Bucket with a lookup handle.
type lookupBucket struct {
	h Handle
}

// NewBucketFromHandle implements the Bucket api with a Lookup handle.
func NewBucketFromHandle(h Handle) bucket.Bucket {
	return &lookupBucket{h: h}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (l *lookupBucket) GetBucketConfig() *bucket.Config {
	return l.h.GetBucketConfig()
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (l *lookupBucket) GetHashType() hash.HashType {
	// NOTE: PutBlock is not implemented by the LookupBucket anyway.
	return 0
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (l *lookupBucket) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrNotImplemented
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (l *lookupBucket) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	lb, err := l.h.GetLookup(ctx)
	if err != nil {
		return nil, false, err
	}
	if lb == nil {
		return nil, false, errors.New("bucket config not found")
	}
	return lb.LookupBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// Note: the block may not be in the specified bucket.
func (l *lookupBucket) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	lb, err := l.h.GetLookup(ctx)
	if err != nil {
		return false, err
	}
	if lb == nil {
		return false, errors.New("bucket config not found")
	}
	_, ok, err := lb.LookupBlock(ctx, ref, WithLocalOnly())
	return ok, err
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (l *lookupBucket) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return ErrNotImplemented
}

// _ is a type assertion
var _ bucket.Bucket = ((*lookupBucket)(nil))
