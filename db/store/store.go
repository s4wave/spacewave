package store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
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

// Store contains all of the Hydra stores.
type Store interface {
	// Execute runs the store's background work until ctx is canceled.
	// Returning nil ends that work and leaves the store in use. Returning an
	// error stops the volume holding the store: the volume controller returns
	// the error from its own Execute.
	Execute(ctx context.Context) error
	// BucketStore is the bucket config store.
	BucketStore
	// BlockStore is the block store.
	BlockStore
	// VolumeStore is the volume store.
	VolumeStore
	// ObjectStore is the object store.
	ObjectStore
}
