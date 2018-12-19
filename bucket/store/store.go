package bucket_store

import (
	"github.com/aperturerobotics/hydra/bucket"
)

// Store is a bucket store.
type Store interface {
	// PutBucketConfig puts a bucket configuration.
	// If outdated, return false, nil
	PutBucketConfig(conf *bucket.Config) (outdated bool, err error)
	// GetLatestBucketConfig gets the bucket config with the highest revision.
	// Can return nil if no bucket config is found.
	GetLatestBucketConfig(id []byte) (*bucket.Config, error)
}
