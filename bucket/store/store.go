package bucket_store

import (
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
	ApplyBucketConfig(conf *bucket.Config) (updated bool, prev, curr *bucket.Config, err error)
	// GetLatestBucketConfig gets the bucket config with the highest revision.
	// Can return nil if no bucket config is found.
	GetLatestBucketConfig(id string) (*bucket.Config, error)
	// GetBucketInfo returns bucket information by string.
	GetBucketInfo(id string) (*bucket.BucketInfo, error)
	// ListBucketInfo lists buckets with an optional regex match.
	ListBucketInfo(idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error)
	// GetReconcilerEventQueue returns a reference to the event queue for a
	// reconciler ID. Should not return nil without an error.
	GetReconcilerEventQueue(BucketReconcilerPair) (mqueue.Queue, error)
	// DeleteReconcilerEventQueue purges a reconciler event queue.
	DeleteReconcilerEventQueue(BucketReconcilerPair) error
	// ListFilledReconcilerEventQueues lists reconciler event queues that have
	// at least one event, by reconciler ID.
	ListFilledReconcilerEventQueues() ([]BucketReconcilerPair, error)
}
