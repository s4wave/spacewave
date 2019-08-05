package store_kvtx

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/kvtx/prefixer"
	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/object/store"
)

// OpenObjectStore opens a object store by ID.
func (k *KVTx) OpenObjectStore(ctx context.Context, id string) (object.ObjectStore, error) {
	prefix := k.kvkey.GetObjectStorePrefixByID(id)
	return kvtx_prefixer.NewPrefixer(k.store, prefix), nil
}

// DelObjectStore deletes a object store and all contents by ID.
func (k *KVTx) DelObjectStore(ctx context.Context, id string) error {
	prefix := k.kvkey.GetObjectStorePrefixByID(id)
	return purge(ctx, kvtx_prefixer.NewPrefixer(k.store, prefix))
}

// purge purges the object store.
func purge(ctx context.Context, store kvtx.Store) error {
	t, err := store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer t.Discard()
	err = t.ScanPrefix(nil, func(key, _ []byte) error {
		return t.Delete(key)
	})
	if err != nil {
		return err
	}
	return t.Commit(ctx)
}

// _ is a type assertion
var _ object_store.Store = ((*KVTx)(nil))
