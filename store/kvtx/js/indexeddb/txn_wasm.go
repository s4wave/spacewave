// +build wasm

package store_kvtx_indexeddb

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/paralin/go-indexeddb"
	"syscall/js"
)

// Tx implements an IndexedDB transaction.
type Tx struct {
	txn         *indexeddb.DurableTransaction
	objStore    *indexeddb.DurableObjectStore
	discardOnce sync.Once
	stringKeys  bool
}

// NewTx constructs a new tranasction, opening the object store.
func NewTx(txn *indexeddb.DurableTransaction, stringKeys bool) (*Tx, error) {
	objStore, err := txn.GetObjectStore(kvStoreObjectStore)
	if err != nil {
		return nil, err
	}

	return &Tx{
		txn:        txn,
		objStore:   objStore,
		stringKeys: stringKeys,
	}, nil
}

// Get returns values for a key.
func (t *Tx) Get(keyb []byte) (data []byte, found bool, err error) {
	key := t.transformKey(keyb)
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
func (t *Tx) Set(keyb, value []byte, ttl time.Duration) error {
	key := t.transformKey(keyb)
	return t.objStore.Put(value, key)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(keyb []byte) error {
	key := t.transformKey(keyb)
	return t.objStore.Delete(key)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, val []byte) error) error {
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
func (t *Tx) Exists(keyb []byte) (bool, error) {
	key := t.transformKey(keyb)
	i, err := t.objStore.Count(key)
	if err != nil {
		return false, err
	}
	return i != 0, nil
}

// transformKey transforms a key as necessary.
func (t *Tx) transformKey(key []byte) interface{} {
	if t.stringKeys {
		return string(key)
	}
	return key
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	t.discardOnce.Do(func() {
		// this prevents abort when calling Discard
	})
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.discardOnce.Do(func() {
		t.txn.Abort()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
