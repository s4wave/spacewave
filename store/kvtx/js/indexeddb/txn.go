//+build js

package kvtx_indexeddb

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/gopherjs/gopherjs/js"
	"github.com/paralin/go-indexeddb"
)

// Tx implements an IndexedDB transaction.
type Tx struct {
	txn         *indexeddb.Transaction
	objStore    *indexeddb.ObjectStore
	discardOnce sync.Once
}

// NewTx constructs a new tranasction, opening the object store.
func NewTx(txn *indexeddb.Transaction) (*Tx, error) {
	objStore, err := txn.GetObjectStore(kvStoreObjectStore)
	if err != nil {
		return nil, err
	}

	return &Tx{
		txn:      txn,
		objStore: objStore,
	}, nil
}

// Get returns values for a key.
func (t *Tx) Get(key []byte) (data []byte, found bool, err error) {
	jsObj, err := t.objStore.Get(key)
	if err != nil {
		return nil, false, err
	}
	if jsObj == js.Undefined {
		return nil, false, nil
	}
	data, ok := jsObj.Interface().([]byte)
	if !ok {
		// for now just ignore the key
		return nil, false, nil
	}
	return data, true, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	// TODO: ttl
	return t.objStore.Put(value, key)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	return t.objStore.Delete(key)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key []byte) error) error {
	krv := js.Undefined
	if len(prefix) != 0 {
		prefixGreater := make([]byte, len(prefix)+1)
		copy(prefixGreater, prefix)
		prefixGreater[len(prefixGreater)-1] = ^byte(0)
		krv = js.Global.Get("IDBKeyRange").Call("bound", prefix, prefixGreater, false, false)
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

		keyBin, ok := val.Key.Interface().([]byte)
		if !ok {
			continue
		}
		if err := cb(keyBin); err != nil {
			return err
		}
		cursor.ContinueCursor()
	}

	return nil
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
