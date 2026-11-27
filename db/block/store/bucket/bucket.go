package block_store_bucket

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
)

// Bucket implements the bucket API backed by a block store.
type Bucket struct {
	block_store.Store
	bucketConf *bucket.Config
}

// NewBucket constructs a new bucket handle.
func NewBucket(blockStore block_store.Store, bucketConf *bucket.Config) *Bucket {
	return &Bucket{
		Store:      blockStore,
		bucketConf: bucketConf,
	}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (b *Bucket) GetBucketConfig() *bucket.Config {
	return b.bucketConf
}

// GetStoredBlockBatch passes the batch read to the block store.
func (b *Bucket) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	return block.GetStoredBlockBatch(ctx, b.Store, refs, fn)
}

// _ is a type assertion
var _ bucket.Bucket = (*Bucket)(nil)
