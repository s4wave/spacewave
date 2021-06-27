// +build wasm

package store_kvtx_indexeddb

import (
	"context"
	"sync"
	"syscall/js"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
	indexeddb "github.com/paralin/go-indexeddb"
)

// kvtxTx implements an IndexedDB transaction.
type kvtxTx struct {
	txn         *indexeddb.DurableTransaction
	objStore    *indexeddb.DurableObjectStore
	discardOnce sync.Once
}

// NewKvtxTx constructs a new tranasction, opening the object store.
func newKvtxTx(txn *indexeddb.DurableTransaction) (*kvtxTx, error) {
	objStore, err := txn.GetObjectStore(kvStoreObjectStore)
	if err != nil {
		return nil, err
	}

	return &kvtxTx{
		txn:      txn,
		objStore: objStore,
	}, nil
}

// Size returns the number of keys in the store.
func (t *kvtxTx) Size() (uint64, error) {
	c, err := t.objStore.Count(nil)
	return uint64(c), err
}

// Get returns values for a key.
func (t *kvtxTx) Get(key []byte) (data []byte, found bool, err error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	jsObj, err := t.objStore.Get(key)
	if err != nil {
		return nil, false, err
	}
	if !jsObj.Truthy() {
		return nil, false, nil
	}
	dlen := jsObj.Length()
	data = make([]byte, dlen)
	js.CopyBytesToGo(data, jsObj)
	return data, true, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *kvtxTx) Set(key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.objStore.Put(value, key)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *kvtxTx) Delete(key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.objStore.Delete(key)
}

// scanPrefix iterates over items with a prefix.
func (t *kvtxTx) scanPrefix(prefix []byte, cb func(v *indexeddb.CursorValue) error) error {
	krv := js.Undefined()
	if len(prefix) != 0 {
		prefixGreater := make([]byte, len(prefix)+1)
		copy(prefixGreater, prefix)
		prefixGreater[len(prefixGreater)-1] = ^byte(0)
		krv = indexeddb.Bound(prefix, prefixGreater, false, false)
	}
	cursor, err := t.objStore.OpenCursor(krv)
	if err != nil {
		return err
	}
	for {
		val := cursor.WaitValue()
		if val == nil {
			return nil
		}

		if err := cb(val); err != nil {
			return err
		}

		cursor.ContinueCursor()
	}
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	return t.scanPrefix(prefix, func(val *indexeddb.CursorValue) error {
		return cb(
			indexeddb.CopyByteSliceFromJs(val.Key),
		)
	})
}

// ScanPrefix iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefix(prefix []byte, cb func(key, val []byte) error) error {
	return t.scanPrefix(prefix, func(val *indexeddb.CursorValue) error {
		return cb(
			indexeddb.CopyByteSliceFromJs(val.Key),
			indexeddb.CopyByteSliceFromJs(val.Value),
		)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
func (t *kvtxTx) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(t, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *kvtxTx) Exists(key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	i, err := t.objStore.Count(key)
	if err != nil {
		return false, err
	}
	return i != 0, nil
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
		txErr = t.txn.Commit()
	})
	return txErr
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *kvtxTx) Discard() {
	t.discardOnce.Do(func() {
		t.txn.Abort()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTx)(nil))
