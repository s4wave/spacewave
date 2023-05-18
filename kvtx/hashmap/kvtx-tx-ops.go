package hashmap

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// kvtxTxOps implements a kvtx tx backed by a hashmap.
type kvtxTxOps struct {
	m               *HashmapKvtx
	commitDiscardFn func(commit bool) error
}

// Get returns values for a key.
func (o *kvtxTxOps) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	return o.m.m.Get(ctx, key)
}

// Size returns number of keys in the store
func (o *kvtxTxOps) Size(ctx context.Context) (uint64, error) {
	return o.m.m.Size(ctx)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (o *kvtxTxOps) Set(ctx context.Context, key, value []byte) error {
	return o.m.m.Set(ctx, key, value)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (o *kvtxTxOps) Delete(ctx context.Context, key []byte) error {
	return o.m.m.Delete(ctx, key)
}

// ScanPrefix iterates over keys with a prefix.
//
// Note: neither key nor value should be retained outside cb() without
// copying.
//
// Note: the ordering of the scan is not necessarily sorted.
func (o *kvtxTxOps) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return o.m.m.Iterate(ctx, func(ctx context.Context, key, dat []byte) error {
		if !bytes.HasPrefix(key, prefix) {
			return nil
		}
		return cb(key, dat)
	})
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (o *kvtxTxOps) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return o.ScanPrefix(ctx, prefix, func(key, value []byte) error {
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
func (o *kvtxTxOps) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(ctx, o, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (o *kvtxTxOps) Exists(ctx context.Context, key []byte) (bool, error) {
	return o.m.m.Exists(ctx, key)
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
		_ = o.commitDiscardFn(false)
	}
}

// _ is a type assertion
var _ kvtx.Tx = ((*kvtxTxOps)(nil))
