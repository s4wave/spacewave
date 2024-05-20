package kvtx_prefixer

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
)

// tx implements a prefixer transaction.
type tx struct {
	lower  kvtx.Tx
	prefix []byte
}

// newTx constructs a new tx with a prefix.
func newTx(lower kvtx.Tx, prefix []byte) *tx {
	return &tx{lower: lower, prefix: prefix}
}

// getKey returns a prefixed key.
func (t *tx) getKey(key []byte) []byte {
	return bytes.Join([][]byte{
		t.prefix,
		key,
	}, nil)
}

// Get returns values for a key.
func (t *tx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	k := t.getKey(key)
	return t.lower.Get(ctx, k)
}

// Size returns the number of keys in the tree.
func (t *tx) Size(ctx context.Context) (uint64, error) {
	return t.lower.Size(ctx)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *tx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	k := t.getKey(key)
	return t.lower.Set(ctx, k, value)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *tx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	k := t.getKey(key)
	return t.lower.Delete(ctx, k)
}

// ScanPrefix iterates over keys with a prefix.
func (t *tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	pfx := t.getKey(prefix)
	return t.lower.ScanPrefix(ctx, pfx, func(key, value []byte) error {
		if !bytes.HasPrefix(key, t.prefix) {
			return nil
		}
		k := key[len(t.prefix):]
		return cb(k, value)
	})
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	pfx := t.getKey(prefix)
	return t.lower.ScanPrefixKeys(ctx, pfx, func(key []byte) error {
		if !bytes.HasPrefix(key, t.prefix) {
			return nil
		}
		k := key[len(t.prefix):]
		return cb(k)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// Iterates in sorted order, reverse reverses the key iteration.
func (t *tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	k := t.getKey(key)
	return t.lower.Exists(ctx, k)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *tx) Commit(ctx context.Context) error {
	return t.lower.Commit(ctx)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *tx) Discard() {
	t.lower.Discard()
}

// _ is a type assertion
var _ kvtx.Tx = ((*tx)(nil))
