package object_store

import (
	"context"

	"github.com/aperturerobotics/hydra/object"
)

// Store implements the object store.
type Store interface {
	// OpenObjectStore opens a object store by ID.
	// The context is used for the API calls.
	OpenObjectStore(ctx context.Context, id string) (object.ObjectStore, error)
	// DelObjectStore deletes a object store and all contents by ID.
	DelObjectStore(ctx context.Context, id string) error
}
