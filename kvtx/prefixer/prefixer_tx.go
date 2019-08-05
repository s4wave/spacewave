package kvtx_prefixer

import (
	"bytes"
	"context"
	"time"

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
func (t *tx) Get(key []byte) (data []byte, found bool, err error) {
	k := t.getKey(key)
	return t.lower.Get(k)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *tx) Set(key, value []byte, ttl time.Duration) error {
	k := t.getKey(key)
	return t.lower.Set(k, value, ttl)

}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *tx) Delete(key []byte) error {
	k := t.getKey(key)
	return t.lower.Delete(k)
}

// ScanPrefix iterates over keys with a prefix.
func (t *tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	pfx := t.getKey(prefix)
	return t.lower.ScanPrefix(pfx, func(key, value []byte) error {
		k := key[len(t.prefix):]
		return cb(k, value)
	})

}

// Exists checks if a key exists.
func (t *tx) Exists(key []byte) (bool, error) {
	k := t.getKey(key)
	return t.lower.Exists(k)
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
