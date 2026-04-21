//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/go-indexeddb/durable"
	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/util/jsbuf"
)

// kvtxTx implements an IndexedDB transaction.
type kvtxTx struct {
	discarded atomic.Bool
	txn       *durable.DurableTransaction
	store     *durable.DurableObjectStore
	write     bool
}

// newKvtxTx constructs a new transaction from a db
func newKvtxTx(db *idb.Database, write bool, objectStoreName string) (*kvtxTx, error) {
	mode := idb.TransactionReadOnly
	if write {
		mode = idb.TransactionReadWrite
	}
	txn, err := durable.NewDurableTransaction(db, mode, objectStoreName)
	if err != nil {
		return nil, err
	}
	store, err := txn.GetObjectStore(objectStoreName)
	if err != nil {
		return nil, err
	}
	return &kvtxTx{txn: txn, store: store, write: write}, nil
}

// Size returns the number of keys in the store.
func (t *kvtxTx) Size(ctx context.Context) (uint64, error) {
	if t.discarded.Load() {
		return 0, kvtx.ErrDiscarded
	}
	resp, err := t.store.Count(ctx)
	return uint64(resp), err
}

// Get returns values for a key.
func (t *kvtxTx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if t.discarded.Load() {
		return nil, false, kvtx.ErrDiscarded
	}

	// convert []byte to js.Value
	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return nil, false, err
	}

	// lookup
	val, err := t.store.Get(ctx, keyVal)
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

	// Update the storage tally before writing.
	if err := t.updateTallyOnSet(ctx, key, value); err != nil {
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

	// NOTE: we use Put instead of Add since Add allows multiple duplicate keys
	return t.store.PutKey(ctx, keyVal, valVal)
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

	// Update the storage tally before deleting.
	if err := t.updateTallyOnDelete(ctx, key); err != nil {
		return err
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}

	return t.store.Delete(ctx, keyVal)
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	it := t.Iterate(ctx, prefix, false, false)
	for {
		if err := it.Err(); err != nil {
			return err
		}
		if !it.Next() {
			break
		}
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefix iterates over keys with a prefix.
func (t *kvtxTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, val []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	it := t.Iterate(ctx, prefix, false, false)
	for {
		if err := it.Err(); err != nil {
			return err
		}
		if !it.Next() {
			break
		}
		key := it.Key()

		val, err := it.Value()
		if err != nil {
			return err
		}

		if err := cb(key, val); err != nil {
			return err
		}
	}
	if err := it.Err(); err != nil {
		return err
	}
	return nil
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse MAY have no effect.
// Must call Next() or Seek() before valid.
func (t *kvtxTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	if t.discarded.Load() {
		return kvtx.NewErrIterator(kvtx.ErrDiscarded)
	}
	// NOTE: IndexedDB cursors are always sorted.
	return BuildKvtxIterator(ctx, t.store, prefix, reverse)
}

// Exists checks if a key exists.
func (t *kvtxTx) Exists(ctx context.Context, key []byte) (bool, error) {
	if t.discarded.Load() {
		return false, kvtx.ErrDiscarded
	}
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return false, err
	}
	count, err := t.store.CountKey(ctx, keyVal)
	if err != nil {
		return false, err
	}
	return count > 0, err
}

// Commit commits the transaction.
func (t *kvtxTx) Commit(ctx context.Context) error {
	if t.discarded.Swap(true) {
		return kvtx.ErrDiscarded
	}
	return t.txn.Commit()
}

// Discard discards the transaction.
func (t *kvtxTx) Discard() {
	t.discarded.Store(true)
	_, _ = t.txn.Abort()
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTx)(nil))
