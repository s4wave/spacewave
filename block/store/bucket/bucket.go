package block_store_bucket

import (
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/bucket"
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

// _ is a type assertion
var _ bucket.Bucket = (*Bucket)(nil)
