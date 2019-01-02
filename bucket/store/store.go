package bucket_store

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/store/mqueue"
	"regexp"
)

// BucketReconcilerPair is a pair of bucket ID and reconciler ID.
type BucketReconcilerPair struct {
	BucketID     string
	ReconcilerID string
}

// Store is a bucket store.
type Store interface {
	// PutBucketConfig puts a bucket configuration.
	// Returns the previous and current (updated) configurations.
	// The current configuration may be nil if the volume rejects the bucket.
	// If outdated, prev == curr.
	PutBucketConfig(conf *bucket.Config) (updated bool, prev, curr *bucket.Config, err error)
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
	// PutBlock puts a block into the store.
	// Stores should check if the block already exists if possible.
	PutBlock(ref *cid.BlockRef, data []byte) (existed bool, err error)
	// GetBlock looks up a block in the store.
	// Returns data, found, and any exceptional error.
	GetBlock(ref *cid.BlockRef) ([]byte, bool, error)
	// RmBlock deletes a block from the store.
	// Should not return an error if the block did not exist.
	RmBlock(ref *cid.BlockRef) error
}
