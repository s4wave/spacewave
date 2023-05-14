package bucket_store

import (
	"context"
	"regexp"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/mqueue"
)

// BucketReconcilerPair is a pair of bucket ID and reconciler ID.
type BucketReconcilerPair struct {
	BucketID     string
	ReconcilerID string
}

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
	// GetReconcilerEventQueue returns a reference to the event queue for a
	// reconciler ID. Should not return nil without an error.
	GetReconcilerEventQueue(context.Context, BucketReconcilerPair) (mqueue.Queue, error)
	// DeleteReconcilerEventQueue purges a reconciler event queue.
	DeleteReconcilerEventQueue(context.Context, BucketReconcilerPair) error
	// ListFilledReconcilerEventQueues lists reconciler event queues that have
	// at least one event, by reconciler ID.
	ListFilledReconcilerEventQueues(ctx context.Context) ([]BucketReconcilerPair, error)
}
