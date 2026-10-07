package volume_scoped

import (
	"context"

	"github.com/s4wave/spacewave/db/object"
	object_store "github.com/s4wave/spacewave/db/object/store"
)

// objectStore serves the object stores of a scoped volume view.
//
// Every object store ID gains the view prefix, so the view reaches only its
// own object stores.
type objectStore struct {
	// inner is the object store of the underlying volume.
	inner object_store.Store
	// prefix is the view prefix.
	prefix string
}

// AccessObjectStore accesses an object store of the view by ID.
func (o *objectStore) AccessObjectStore(ctx context.Context, id string, released func()) (object.ObjectStore, func(), error) {
	return o.inner.AccessObjectStore(ctx, o.prefix+id, released)
}

// DeleteObjectStore deletes an object store of the view and all contents by ID.
func (o *objectStore) DeleteObjectStore(ctx context.Context, id string) error {
	return o.inner.DeleteObjectStore(ctx, o.prefix+id)
}

// _ is a type assertion
var _ object_store.Store = (*objectStore)(nil)
