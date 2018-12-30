package store

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/volume/store"
)

// BucketStore is the bucket store.
type BucketStore = bucket_store.Store

// VolumeStore is the volume store.
type VolumeStore = volume_store.Store

// Store contains all of the Hydra stores.
type Store interface {
	// Execute executes the given store.
	// Returning nil ends execution.
	// Returning an error triggers a retry with backoff.
	Execute(ctx context.Context) error
	// GetStoreID returns the store identifier.
	// Format: hydra/badger/1 or similar.
	GetStoreID() string
	// BucketStore is the bucket store.
	BucketStore
	// VolumeStore is the volume store.
	VolumeStore
}
