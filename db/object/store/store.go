package object_store

import (
	"context"

	"github.com/s4wave/spacewave/db/object"
)

// Store implements the object store.
type Store interface {
	// AccessObjectStore accesses a object store by ID.
	// The context is used for the API calls.
	// Returns theh object store, a release function.
	// Accepts a function to call if the ObjectStore is released.
	AccessObjectStore(ctx context.Context, id string, released func()) (object.ObjectStore, func(), error)
	// DeleteObjectStore deletes a object store and all contents by ID.
	// Existing object store handles will be released.
	DeleteObjectStore(ctx context.Context, id string) error
}
