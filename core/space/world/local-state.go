package space_world

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// readLocal returns the value of key in the SharedObject's local state store
// storeID, and whether it is set.
func (a *Adapter) readLocal(ctx context.Context, storeID string, key []byte) ([]byte, bool, error) {
	// Open the store.
	store, release, err := a.so.AccessLocalStateStore(ctx, storeID, nil)
	if err != nil {
		return nil, false, err
	}
	defer release()

	// Read a copy of the value, which outlives the transaction.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, key)
	return bytes.Clone(data), found, err
}

// writeLocal sets key to value in the SharedObject's local state store
// storeID, or deletes it when value is nil.
func (a *Adapter) writeLocal(ctx context.Context, storeID string, key, value []byte) error {
	// Open the store.
	store, release, err := a.so.AccessLocalStateStore(ctx, storeID, nil)
	if err != nil {
		return err
	}
	defer release()

	// Set or delete the key.
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }
	return kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		if value == nil {
			return tx.Delete(ctx, key)
		}
		return tx.Set(ctx, key, value)
	})
}
