//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/util/jsbuf"
	"github.com/hack-pad/safejs"
)

// kvtxTx implements an IndexedDB transaction.
type kvtxTx struct {
	tx          *idb.Transaction
	store       *idb.ObjectStore
	discardOnce sync.Once
}

// newKvtxTx constructs a new transaction, opening the object store.
func newKvtxTx(txn *idb.Transaction) (*kvtxTx, error) {
	store, err := txn.ObjectStore(kvStoreObjectStore)
	if err != nil {
		return nil, err
	}
	return &kvtxTx{
		tx:    txn,
		store: store,
	}, nil
}

// Size returns the number of keys in the store.
func (t *kvtxTx) Size(ctx context.Context) (uint64, error) {
	resp, err := t.store.Count()
	if err != nil {
		return 0, err
	}

	num, err := resp.Await(ctx)
	return uint64(num), err
}

// Get returns values for a key.
func (t *kvtxTx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	// convert []byte to js.Value
	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return nil, false, err
	}

	// lookup
	req, err := t.store.Get(keyVal)
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
// This will not be committed until Commit is called.
func (t *kvtxTx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}

	valVal, err := jsbuf.CopyBytesToJs(value)
	if err != nil {
		return err
	}

	req, err := t.store.PutKey(keyVal, valVal)
	if err != nil {
		return err
	}

	_, err = req.Await(ctx)
	return err
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *kvtxTx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}

	req, err := t.store.Delete(safejs.Unsafe(keyVal))
	if err != nil {
		return err
	}

	return req.Await(ctx)
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	prefixVal, err := jsbuf.CopyBytesToJs(prefix)
	if err != nil {
		return err
	}

	keyRange, err := idb.NewKeyRangeLowerBound(prefixVal, false)
	if err != nil {
		return err
	}

	req, err := t.store.OpenKeyCursorRange(keyRange, idb.CursorNext)
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
	prefixVal, err := jsbuf.CopyBytesToJs(prefix)
	if err != nil {
		return err
	}

	keyRange, err := idb.NewKeyRangeLowerBound(prefixVal, false)
	if err != nil {
		return err
	}

	req, err := t.store.OpenCursorRange(keyRange, idb.CursorNext)
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
	return BuildKvtxIterator(ctx, t.store, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *kvtxTx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return false, err
	}
	req, err := t.store.CountKey(keyVal)
	if err != nil {
		return false, err
	}

	count, err := req.Await(ctx)
	return count > 0, err
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *kvtxTx) Commit(ctx context.Context) error {
	// Note that commit() doesn't normally have to be called — a transaction
	// will automatically commit when all outstanding requests have been
	// satisfied and no new requests have been made. commit() can be used to
	// start the commit process without waiting for events from outstanding
	// requests to be dispatched.
	var txErr error
	t.discardOnce.Do(func() {
		txErr = t.tx.Commit()
	})
	return txErr
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *kvtxTx) Discard() {
	t.discardOnce.Do(func() {
		_ = t.tx.Abort()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTx)(nil))
