// +build wasm

package store_kvtx_indexeddb

import (
	"context"
	"sync"
	"time"

	"syscall/js"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/paralin/go-indexeddb"
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

// Get returns values for a key.
func (t *kvtxTx) Get(key []byte) (data []byte, found bool, err error) {
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
func (t *kvtxTx) Set(key, value []byte, ttl time.Duration) error {
	return t.objStore.Put(value, key)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *kvtxTx) Delete(key []byte) error {
	return t.objStore.Delete(key)
}

// ScanPrefix iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefix(prefix []byte, cb func(key, val []byte) error) error {
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
			break
		}

		if err := cb(
			indexeddb.CopyByteSliceFromJs(val.Key),
			indexeddb.CopyByteSliceFromJs(val.Value),
		); err != nil {
			return err
		}
		cursor.ContinueCursor()
	}

	return nil
}

// Exists checks if a key exists.
func (t *kvtxTx) Exists(key []byte) (bool, error) {
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
