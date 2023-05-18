package kvtx_txcache

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	tc := t.tc
	if tc == nil {
		return nil, false, kvtx.ErrDiscarded
	}
	return tc.Get(ctx, key)
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	tc := t.tc
	if tc == nil {
		return 0, kvtx.ErrDiscarded
	}
	return tc.Size(ctx)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return tc.Set(ctx, key, value)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return tc.Delete(ctx, key)
}

// ScanPrefix iterates over keys with a prefix.
//
// Note: neither key nor value should be retained outside cb() without
// copying.
//
// Note: the ordering of the scan is not necessarily sorted.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return tc.ScanPrefix(ctx, prefix, cb)
}

// ScanPrefixKeys iterates over keys with a prefix.
//
// To enforce ordering, it builds a set in memory, sorts, then operates.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return t.ScanPrefixKeys(ctx, prefix, cb)
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// Iterates in sorted order, reverse reverses the key iteration.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	tc := t.tc
	if tc == nil {
		return kvtx.NewErrIterator(kvtx.ErrDiscarded)
	}
	return t.tc.Iterate(ctx, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	tc := t.tc
	if tc == nil {
		return false, kvtx.ErrDiscarded
	}
	return tc.Exists(ctx, key)
}

// _ is a type assertion
var _ kvtx.TxOps = ((*Tx)(nil))
