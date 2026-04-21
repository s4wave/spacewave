package store_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/object"
	object_store "github.com/s4wave/spacewave/db/object/store"
)

// AccessObjectStore accesses an object store by ID.
func (k *KVTx) AccessObjectStore(ctx context.Context, id string, released func()) (object.ObjectStore, func(), error) {
	prefix := k.kvkey.GetObjectStorePrefixByID(id)
	return kvtx_prefixer.NewPrefixer(k.store, prefix), func() {}, nil
}

// DeleteObjectStore deletes a object store and all contents by ID.
func (k *KVTx) DeleteObjectStore(ctx context.Context, id string) error {
	prefix := k.kvkey.GetObjectStorePrefixByID(id)
	return purge(ctx, kvtx_prefixer.NewPrefixer(k.store, prefix))
}

// purge purges the object store.
func purge(ctx context.Context, store kvtx.Store) error {
	t, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer t.Discard()
	err = t.ScanPrefix(ctx, nil, func(key, _ []byte) error {
		return t.Delete(ctx, key)
	})
	if err != nil {
		return err
	}
	return t.Commit(ctx)
}

// _ is a type assertion
var _ object_store.Store = ((*KVTx)(nil))
