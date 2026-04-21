package store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	mqueue_store "github.com/s4wave/spacewave/db/mqueue/store"
	object_store "github.com/s4wave/spacewave/db/object/store"
	volume_store "github.com/s4wave/spacewave/db/volume/store"
)

// BucketStore is the bucket config store.
type BucketStore = bucket_store.Store

// BlockStore is the block store.
type BlockStore = block.StoreOps

// VolumeStore is the volume store.
type VolumeStore = volume_store.Store

// ObjectStore is the object store.
type ObjectStore = object_store.Store

// MqueueStore is the message queue store.
type MqueueStore = mqueue_store.Store

// Store contains all of the Hydra stores.
type Store interface {
	// Execute executes the given store.
	// Returning nil ends execution.
	// Returning an error triggers a retry with backoff.
	Execute(ctx context.Context) error
	// BucketStore is the bucket config store.
	BucketStore
	// BlockStore is the block store.
	BlockStore
	// VolumeStore is the volume store.
	VolumeStore
	// ObjectStore is the object store.
	ObjectStore
	// MqueueStore is the message queue store.
	MqueueStore
}
