//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/util/jsbuf"
	"github.com/hack-pad/safejs"
)

// kvtxTx implements an IndexedDB transaction.
type kvtxTx struct {
	discarded atomic.Bool
	db        *idb.Database
	write     bool
}

// newKvtxTx constructs a new transaction from a db
func newKvtxTx(db *idb.Database, write bool) *kvtxTx {
	return &kvtxTx{db: db, write: write}
}

// Size returns the number of keys in the store.
func (t *kvtxTx) Size(ctx context.Context) (uint64, error) {
	if t.discarded.Load() {
		return 0, kvtx.ErrDiscarded
	}

	txn, err := t.db.Transaction(idb.TransactionReadOnly, kvStoreObjectStore)
	if err != nil {
		return 0, err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return 0, err
	}

	resp, err := store.Count()
	if err != nil {
		return 0, err
	}

	num, err := resp.Await(ctx)
	return uint64(num), err
}

// Get returns values for a key.
func (t *kvtxTx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if t.discarded.Load() {
		return nil, false, kvtx.ErrDiscarded
	}

	txn, err := t.db.Transaction(idb.TransactionReadOnly, kvStoreObjectStore)
	if err != nil {
		return nil, false, err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return nil, false, err
	}

	// convert []byte to js.Value
	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return nil, false, err
	}

	// lookup
	req, err := store.Get(keyVal)
	if err != nil {
		return nil, false, err
	}

	val, err := req.Await(ctx)
	if err != nil {
		return nil, false, err
	}

	// not found
	if val.IsNull() || val.IsUndefined() {
		return nil, false, nil
	}

	data, err = jsbuf.CopyBytesToGo(val)
	if err != nil {
		return nil, false, err
	}

	return data, true, nil
}

// Set sets the value of a key.
func (t *kvtxTx) Set(ctx context.Context, key, value []byte) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	txn, err := t.db.Transaction(idb.TransactionReadWrite, kvStoreObjectStore)
	if err != nil {
		return err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return err
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}

	valVal, err := jsbuf.CopyBytesToJs(value)
	if err != nil {
		return err
	}

	req, err := store.PutKey(keyVal, valVal)
	if err != nil {
		return err
	}

	_, err = req.Await(ctx)
	return err
}

// Delete deletes a key.
func (t *kvtxTx) Delete(ctx context.Context, key []byte) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	txn, err := t.db.Transaction(idb.TransactionReadWrite, kvStoreObjectStore)
	if err != nil {
		return err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return err
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}

	req, err := store.Delete(safejs.Unsafe(keyVal))
	if err != nil {
		return err
	}

	return req.Await(ctx)
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	txn, err := t.db.Transaction(idb.TransactionReadOnly, kvStoreObjectStore)
	if err != nil {
		return err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return err
	}

	prefixVal, err := jsbuf.CopyBytesToJs(prefix)
	if err != nil {
		return err
	}

	keyRange, err := idb.NewKeyRangeLowerBound(prefixVal, false)
	if err != nil {
		return err
	}

	req, err := store.OpenKeyCursorRange(keyRange, idb.CursorNext)
	if err != nil {
		return err
	}

	return req.Iter(ctx, func(cursor *idb.Cursor) error {
		keyVal, err := cursor.Key()
		if err != nil {
			return err
		}

		key, err := jsbuf.CopyBytesToGo(keyVal)
		if err != nil {
			return err
		}

		// Stop iterating if the key no longer has the prefix
		if !bytes.HasPrefix(key, prefix) {
			return idb.ErrCursorStopIter
		}

		return cb(key)
	})
}

// ScanPrefix iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, val []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	txn, err := t.db.Transaction(idb.TransactionReadOnly, kvStoreObjectStore)
	if err != nil {
		return err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return err
	}

	prefixVal, err := jsbuf.CopyBytesToJs(prefix)
	if err != nil {
		return err
	}

	keyRange, err := idb.NewKeyRangeLowerBound(prefixVal, false)
	if err != nil {
		return err
	}

	req, err := store.OpenCursorRange(keyRange, idb.CursorNext)
	if err != nil {
		return err
	}

	return req.Iter(ctx, func(cursor *idb.CursorWithValue) error {
		keyVal, err := cursor.Key()
		if err != nil {
			return err
		}

		key, err := jsbuf.CopyBytesToGo(keyVal)
		if err != nil {
			return err
		}

		// Stop iterating if the key no longer has the prefix
		if !bytes.HasPrefix(key, prefix) {
			return idb.ErrCursorStopIter
		}

		valVal, err := cursor.Value()
		if err != nil {
			return err
		}

		val, err := jsbuf.CopyBytesToGo(valVal)
		if err != nil {
			return err
		}

		return cb(key, val)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
func (t *kvtxTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	if t.discarded.Load() {
		return kvtx.NewErrIterator(kvtx.ErrDiscarded)
	}

	txn, err := t.db.Transaction(idb.TransactionReadOnly, kvStoreObjectStore)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	return BuildKvtxIterator(ctx, store, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *kvtxTx) Exists(ctx context.Context, key []byte) (bool, error) {
	if t.discarded.Load() {
		return false, kvtx.ErrDiscarded
	}
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}

	txn, err := t.db.Transaction(idb.TransactionReadOnly, kvStoreObjectStore)
	if err != nil {
		return false, err
	}
	defer txn.Commit()

	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return false, err
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return false, err
	}

	req, err := store.CountKey(keyVal)
	if err != nil {
		return false, err
	}

	count, err := req.Await(ctx)
	return count > 0, err
}

// Commit commits the transaction.
func (t *kvtxTx) Commit(ctx context.Context) error {
	if t.discarded.Swap(true) {
		return kvtx.ErrDiscarded
	}
	return nil
}

// Discard discards the transaction.
func (t *kvtxTx) Discard() {
	t.discarded.Store(true)
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTx)(nil))
