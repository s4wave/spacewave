//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
	indexeddb "github.com/paralin/go-indexeddb"
)

// kvtxTx implements an IndexedDB transaction.
type kvtxTx struct {
	tx          *indexeddb.Kvtx
	discardOnce sync.Once
}

// NewKvtxTx constructs a new tranasction, opening the object store.
func newKvtxTx(txn *indexeddb.DurableTransaction) (*kvtxTx, error) {
	tx, err := indexeddb.NewKvtxTx(txn, kvStoreObjectStore)
	if err != nil {
		return nil, err
	}
	return &kvtxTx{tx: tx}, nil
}

// Size returns the number of keys in the store.
func (t *kvtxTx) Size() (uint64, error) {
	return t.tx.Size()
}

// Get returns values for a key.
func (t *kvtxTx) Get(key []byte) (data []byte, found bool, err error) {
	return t.tx.Get(key)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *kvtxTx) Set(key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.tx.Set(key, value)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *kvtxTx) Delete(key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.tx.Delete(key)
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	return t.tx.ScanPrefixKeys(prefix, cb)
}

// ScanPrefix iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefix(prefix []byte, cb func(key, val []byte) error) error {
	return t.tx.ScanPrefix(prefix, cb)
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
	return t.tx.Exists(key)
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
		t.tx.Discard()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTx)(nil))
