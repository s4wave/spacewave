package lookup_concurrent

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// BeginReadOperation retains the sole local bucket's read scope when available.
// Network fallback remains live; writeback lookups keep their unscoped path.
func (c *LookupController) BeginReadOperation(ctx context.Context) (lookup.Lookup, func(), error) {
	// Resolve the controller's bucket handles.
	handles, err := c.getBucketHandles(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Multi-bucket lookup can return while losing reads are still running.
	// Those reads cannot safely share a caller-released storage scope.
	if len(handles) != 1 {
		return c, func() {}, nil
	}

	// A native read scope may block writes, including synchronous writeback.
	if c.conf.GetWritebackBehavior() != WritebackBehavior_WritebackBehavior_NONE {
		return c, func() {}, nil
	}

	// Open a read scope on the sole local bucket.
	original := handles[0].GetBucket()
	read, release, err := original.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	readBucket := &readScopeBucket{StoreOps: read, conf: original.GetBucketConfig()}
	scopedHandles := []bucket.BucketHandle{bucket.NewBucketHandle(
		handles[0].GetID(), readBucket,
	)}

	// Retain lookup policy and fallback ownership, replacing only the local handle.
	scoped := *c
	scoped.conf = c.conf.CloneVT()
	scoped.conf.PutBlockBehavior = PutBlockBehavior_PutBlockBehavior_NONE
	scoped.bucketHandleSetCtr = ccontainer.NewCContainer(&scopedHandles)
	return &scoped, release, nil
}

// readScopeBucket supplies bucket metadata for an existing scoped block store.
type readScopeBucket struct {
	// StoreOps contains the underlying bucket's bounded read operations.
	block.StoreOps
	// conf identifies the bucket retained by the operation.
	conf *bucket.Config
}

// GetBucketConfig returns the retained bucket configuration.
func (b *readScopeBucket) GetBucketConfig() *bucket.Config {
	return b.conf
}

// _ is a type assertion.
var _ bucket.Bucket = (*readScopeBucket)(nil)
