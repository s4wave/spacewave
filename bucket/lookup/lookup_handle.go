package bucket_lookup

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
)

var (
	// ErrNotImplemented is returned for operations not implemented by Lookup().
	ErrNotImplemented = errors.New("operation not implemented by lookup controller")
)

// lookupBucket implements bucket.Bucket with a lookup handle.
type lookupBucket struct {
	ctx context.Context
	h   Handle
}

// NewBucketFromHandle implements the Bucket api with a Lookup handle.
func NewBucketFromHandle(ctx context.Context, h Handle) bucket.Bucket {
	return &lookupBucket{ctx: ctx, h: h}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (l *lookupBucket) GetBucketConfig() *bucket.Config {
	return l.h.GetBucketConfig()
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (l *lookupBucket) PutBlock(data []byte, opts *bucket.PutOpts) (*bucket_event.PutBlock, error) {
	return nil, ErrNotImplemented
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (l *lookupBucket) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
	lb, err := l.h.GetLookup(l.ctx)
	if err != nil {
		return nil, false, err
	}
	if lb == nil {
		return nil, false, errors.New("bucket config not found")
	}
	return lb.LookupBlock(l.ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// Note: the block may not be in the specified bucket.
func (l *lookupBucket) GetBlockExists(ref *cid.BlockRef) (bool, error) {
	lb, err := l.h.GetLookup(l.ctx)
	if err != nil {
		return false, err
	}
	if lb == nil {
		return false, errors.New("bucket config not found")
	}
	_, ok, err := lb.LookupBlock(l.ctx, ref, WithLocalOnly())
	return ok, err
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (l *lookupBucket) RmBlock(ref *cid.BlockRef) error {
	return ErrNotImplemented
}

// _ is a type assertion
var _ bucket.Bucket = ((*lookupBucket)(nil))
