package bucket_lookup

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
)

// ErrNotImplemented is returned for operations not implemented by Lookup().
var ErrNotImplemented = errors.New("operation not implemented by lookup controller")

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

// GetSupportedFeatures returns the native feature bitmask for the store.
func (l *lookupBucket) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (l *lookupBucket) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	lb, err := l.h.GetLookup(ctx)
	if err != nil {
		return nil, false, err
	}
	if lb == nil {
		return nil, false, bucket.ErrBucketNotFound
	}

	var blockRef *block.BlockRef
	objRefs, existed, err := lb.PutBlock(ctx, data, opts)
	for _, objRef := range objRefs {
		rootRef := objRef.GetRootRef()
		if !rootRef.GetEmpty() {
			blockRef = rootRef
			break
		}
	}
	return blockRef, existed, err
}

// PutBlockBatch loops calling PutBlock or RmBlock per entry.
func (l *lookupBucket) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			if err := l.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		var ref *block.BlockRef
		if entry.Ref != nil {
			ref = entry.Ref.Clone()
		}
		if _, _, err := l.PutBlock(ctx, entry.Data, &block.PutOpts{
			ForceBlockRef: ref,
			Refs:          block.CloneBlockRefs(entry.Refs),
		}); err != nil {
			return err
		}
	}
	return nil
}

// PutBlockBackground forwards to PutBlock.
func (l *lookupBucket) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return l.PutBlock(ctx, data, opts)
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
		return nil, false, bucket.ErrBucketNotFound
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
		return false, bucket.ErrBucketNotFound
	}
	_, ok, err := lb.LookupBlock(ctx, ref, WithLocalOnly())
	return ok, err
}

// GetBlockExistsBatch loops calling GetBlockExists per ref.
func (l *lookupBucket) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := l.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (l *lookupBucket) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	found, err := l.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: -1}, nil
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (l *lookupBucket) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return ErrNotImplemented
}

// Flush returns nil because lookupBucket has no buffered writes.
func (l *lookupBucket) Flush(context.Context) error {
	return nil
}

// BeginDeferFlush opens a no-op defer-flush scope.
func (l *lookupBucket) BeginDeferFlush() {}

// EndDeferFlush closes a no-op defer-flush scope.
func (l *lookupBucket) EndDeferFlush(context.Context) error {
	return nil
}

// _ is a type assertion
var _ bucket.Bucket = ((*lookupBucket)(nil))
