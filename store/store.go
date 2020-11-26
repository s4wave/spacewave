package store

import (
	"context"

	block_store "github.com/aperturerobotics/hydra/block/store"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	object_store "github.com/aperturerobotics/hydra/object/store"
	volume_store "github.com/aperturerobotics/hydra/volume/store"
)

// BucketStore is the bucket config store.
type BucketStore = bucket_store.Store

// BlockStore is the block store.
type BlockStore = block_store.Store

// VolumeStore is the volume store.
type VolumeStore = volume_store.Store

// ObjectStore is the object store.
type ObjectStore = object_store.Store

// Store contains all of the Hydra stores.
type Store interface {
	// Execute executes the given store.
	// Returning nil ends execution.
	// Returning an error triggers a retry with backoff.
	Execute(ctx context.Context) error
	// GetStoreID returns the store identifier.
	// Format: hydra/badger/1 or similar.
	GetStoreID() string
	// BucketStore is the bucket config store.
	BucketStore
	// BlockStore is the block store.
	BlockStore
	// VolumeStore is the volume store.
	VolumeStore
	// ObjectStore is the object store.
	ObjectStore
}
