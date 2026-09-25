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
	// h resolves the bucket's lookup service.
	h Handle
	// lookup retains the local bucket scopes of a bounded read operation.
	lookup Lookup
	// localOnly excludes remote discovery from payload reads.
	localOnly bool
}

// NewBucketFromHandle exposes the bucket API through a lookup handle.
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

// BeginReadOperation retains the lookup's local bucket read scopes.
func (l *lookupBucket) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	if l.lookup != nil {
		return l, func() {}, nil
	}
	lookup, err := l.h.GetLookup(ctx)
	if err != nil {
		return nil, nil, err
	}
	if lookup == nil {
		return nil, nil, bucket.ErrBucketNotFound
	}
	lookup, release, err := lookup.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scoped := *l
	scoped.lookup = lookup
	return &scoped, release, nil
}

// getLookup uses retained bucket scopes when the caller opened a read operation.
func (l *lookupBucket) getLookup(ctx context.Context) (Lookup, error) {
	if l.lookup != nil {
		return l.lookup, nil
	}
	return l.h.GetLookup(ctx)
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (l *lookupBucket) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Resolve the lookup handle before writing the block.
	lb, err := l.getLookup(ctx)
	if err != nil {
		return nil, false, err
	}
	if lb == nil {
		return nil, false, bucket.ErrBucketNotFound
	}

	var blockRef *block.BlockRef

	// Select the first non-empty root reference returned by the lookup write.
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
	// Apply each batch entry as either a tombstone removal or a block write.
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

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (l *lookupBucket) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	lb, err := l.getLookup(ctx)
	if err != nil {
		return nil, false, err
	}
	if lb == nil {
		return nil, false, bucket.ErrBucketNotFound
	}
	if l.localOnly {
		return lb.LookupBlock(ctx, ref, WithLocalOnly())
	}
	return lb.LookupBlock(ctx, ref)
}

// GetStoredBlock looks up a block and its refs with the lookup controller.
func (l *lookupBucket) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	lb, err := l.getLookup(ctx)
	if err != nil {
		return nil, err
	}
	if lb == nil {
		return nil, bucket.ErrBucketNotFound
	}
	if l.localOnly {
		return lb.LookupStoredBlock(ctx, ref, WithLocalOnly())
	}
	return lb.LookupStoredBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// Note: the block may not be in the specified bucket.
func (l *lookupBucket) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	found, err := l.GetBlockExistsBatch(ctx, []*block.BlockRef{ref})
	if err != nil {
		return false, err
	}
	return found[0], nil
}

// GetBlockExistsBatch checks whether refs exist through the lookup controller.
func (l *lookupBucket) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	lb, err := l.getLookup(ctx)
	if err != nil {
		return nil, err
	}
	if lb == nil {
		return nil, bucket.ErrBucketNotFound
	}
	return lb.LookupBlockExistsBatch(ctx, refs, WithLocalOnly())
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

// Sync reports always-durable: lookupBucket holds no buffered writes.
func (l *lookupBucket) Sync(context.Context) (bool, error) {
	return true, nil
}

// _ verifies the bucket contract.
var _ bucket.Bucket = (*lookupBucket)(nil)
