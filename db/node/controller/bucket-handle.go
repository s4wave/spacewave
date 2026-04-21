package node_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// bucketLookupHandle implements a bucket lookup handle value.
type bucketLookupHandle struct {
	b *loadedBucket
	s *loadedBucketState
}

// newBucketLookupHandle builds a new bucketLookupHandle
func newBucketLookupHandle(b *loadedBucket, s *loadedBucketState) *bucketLookupHandle {
	return &bucketLookupHandle{b: b, s: s}
}

// GetDisposed returns if this bucket handle is disposed.
func (c *bucketLookupHandle) GetDisposed() bool {
	return c.s.disposed
}

// GetBucketConfig returns the current in-use bucket config.
// Will be nil if the bucket is not known.
func (c *bucketLookupHandle) GetBucketConfig() *bucket.Config {
	return c.s.info.GetConfig()
}

// GetLookup returns the lookup handle.
// Will return nil if the bucket config is not yet known.
func (c *bucketLookupHandle) GetLookup(
	ctx context.Context,
) (bucket_lookup.Lookup, error) {
	if c.s.info.GetConfig() == nil {
		return nil, nil
	}

	return c.b.GetLookup(ctx)
}

// _ is a type assertion
var _ bucket_lookup.Handle = ((*bucketLookupHandle)(nil))
