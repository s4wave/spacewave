package space_world

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// readLocal returns the value of key in the SharedObject's local state store
// storeID, and whether it is set.
func (a *Adapter) readLocal(ctx context.Context, storeID string, key []byte) ([]byte, bool, error) {
	return readLocalState(ctx, a.so, storeID, key)
}

// readLocalState returns the value of key in the local state store storeID
// of so, and whether it is set.
func readLocalState(ctx context.Context, so sobject.SharedObject, storeID string, key []byte) ([]byte, bool, error) {
	// Open the store.
	store, release, err := so.AccessLocalStateStore(ctx, storeID, nil)
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
	return writeLocalState(ctx, a.so, storeID, key, value)
}

// writeLocalState sets key to value in the local state store storeID of so,
// or deletes it when value is nil.
func writeLocalState(ctx context.Context, so sobject.SharedObject, storeID string, key, value []byte) error {
	// Open the store.
	store, release, err := so.AccessLocalStateStore(ctx, storeID, nil)
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
