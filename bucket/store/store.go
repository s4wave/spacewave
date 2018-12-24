package bucket_store

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/store/mqueue"
)

// BucketReconcilerPair is a pair of bucket ID and reconciler ID.
type BucketReconcilerPair struct {
	BucketID     string
	ReconcilerID string
}

// Store is a bucket store.
type Store interface {
	// PutBucketConfig puts a bucket configuration.
	// If outdated, return false, nil
	PutBucketConfig(conf *bucket.Config) (outdated bool, err error)
	// GetLatestBucketConfig gets the bucket config with the highest revision.
	// Can return nil if no bucket config is found.
	GetLatestBucketConfig(id string) (*bucket.Config, error)
	// GetReconcilerEventQueue returns a reference to the event queue for a
	// reconciler ID. Should not return nil without an error.
	GetReconcilerEventQueue(BucketReconcilerPair) (mqueue.Queue, error)
	// DeleteReconcilerEventQueue purges a reconciler event queue.
	DeleteReconcilerEventQueue(BucketReconcilerPair) error
	// ListFilledReconcilerEventQueues lists reconciler event queues that have
	// at least one event, by reconciler ID.
	ListFilledReconcilerEventQueues() ([]BucketReconcilerPair, error)
}
