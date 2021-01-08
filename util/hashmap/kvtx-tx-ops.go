package hashmap

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// kvtxTxOps implements a kvtx tx backed by a hashmap.
type kvtxTxOps struct {
	m               *HashmapKvtx
	commitDiscardFn func(commit bool) error
}

// Get returns values for a key.
func (o *kvtxTxOps) Get(key []byte) (data []byte, found bool, err error) {
	dat, ok := o.m.m.Get(key)
	if !ok {
		return nil, false, nil
	}
	data, found = dat.([]byte)
	return data, found, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (o *kvtxTxOps) Set(key, value []byte, ttl time.Duration) error {
	o.m.m.Set(key, value)
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (o *kvtxTxOps) Delete(key []byte) error {
	o.m.m.Remove(key)
	return nil
}

// ScanPrefix iterates over keys with a prefix.
//
// Note: neither key nor value should be retained outside cb() without
// copying.
//
// Note: the ordering of the scan is not necessarily sorted.
func (o *kvtxTxOps) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	return o.m.m.Iterate(func(key []byte, value interface{}) error {
		dat, ok := value.([]byte)
		if !ok || !bytes.HasPrefix(key, prefix) {
			return nil
		}
		return cb(key, dat)
	})
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (o *kvtxTxOps) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	return o.ScanPrefix(prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
func (o *kvtxTxOps) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(o, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (o *kvtxTxOps) Exists(key []byte) (bool, error) {
	return o.m.m.Exists(key), nil
}

// _ is a type assertion
var _ kvtx.TxOps = ((*kvtxTxOps)(nil))

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (o *kvtxTxOps) Commit(ctx context.Context) error {
	// noop: we already commit instantly.
	if o.commitDiscardFn != nil {
		return o.commitDiscardFn(true)
	}
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (o *kvtxTxOps) Discard() {
	if o.commitDiscardFn != nil {
		o.commitDiscardFn(false)
	}
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTxOps)(nil))
