package bucket_store

import (
	"context"
	"regexp"

	"github.com/s4wave/spacewave/db/bucket"
)

// Store is a bucket store.
type Store interface {
	// ApplyBucketConfig applies a bucket configuration.
	// Returns the previous and current (updated) configurations.
	// The current configuration may be nil if the volume rejects the bucket.
	// If outdated, prev == curr.
	ApplyBucketConfig(ctx context.Context, conf *bucket.Config) (updated bool, prev, curr *bucket.Config, err error)
	// GetBucketConfig gets the bucket config for the bucket ID.
	// Can return nil if no bucket config is found.
	GetBucketConfig(ctx context.Context, id string) (*bucket.Config, error)
	// GetBucketInfo returns bucket information by bucket ID.
	GetBucketInfo(ctx context.Context, id string) (*bucket.BucketInfo, error)
	// ListBucketInfo lists buckets with an optional regex match.
	ListBucketInfo(ctx context.Context, idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error)
}
