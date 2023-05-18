package volume

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/tx"
)

// busObjectStoreTx implements tx.Tx for a BusObjectStore.
type busObjectStoreTx struct {
	rel    atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
	ref    directive.Reference
	utx    kvtx.Tx
}

// Size returns the number of keys in the store.
func (t *busObjectStoreTx) Size(ctx context.Context) (size uint64, err error) {
	err = t.do(func() (rerr error) {
		size, rerr = t.utx.Size(ctx)
		return
	})
	return
}

// Get returns values for a key.
func (t *busObjectStoreTx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	err = t.do(func() (rerr error) {
		data, found, rerr = t.utx.Get(ctx, key)
		return
	})
	return
}

// Exists checks if a key exists.
func (t *busObjectStoreTx) Exists(ctx context.Context, key []byte) (ex bool, err error) {
	err = t.do(func() (rerr error) {
		ex, rerr = t.utx.Exists(ctx, key)
		return
	})
	return
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *busObjectStoreTx) Set(ctx context.Context, key, value []byte) error {
	return t.do(func() error {
		return t.utx.Set(ctx, key, value)
	})
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *busObjectStoreTx) Delete(ctx context.Context, key []byte) error {
	return t.do(func() error {
		return t.utx.Delete(ctx, key)
	})
}

// ScanPrefix iterates over keys with a prefix.
//
// Note: neither key nor value should be retained outside cb() without
// copying.
//
// Note: the ordering of the scan is not necessarily sorted.
func (t *busObjectStoreTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.do(func() error {
		return t.utx.ScanPrefix(ctx, prefix, cb)
	})
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *busObjectStoreTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.do(func() error {
		return t.utx.ScanPrefixKeys(ctx, prefix, cb)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
// Some implementations return BlockIterator.
func (t *busObjectStoreTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	if t.rel.Load() {
		return kvtx.NewErrIterator(tx.ErrDiscarded)
	}
	return t.utx.Iterate(ctx, prefix, sort, reverse)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *busObjectStoreTx) Commit(ctx context.Context) error {
	if t.rel.Load() {
		return tx.ErrDiscarded
	}
	err := t.utx.Commit(ctx)
	t.Discard() // discard after commit to ensure we cleanup fully
	return err
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *busObjectStoreTx) Discard() {
	t.rel.Store(true)
	t.utx.Discard()
	t.cancel()
	t.ref.Release()
}

// do performs an operation with a release check and a discarded cleanup check.
func (t *busObjectStoreTx) do(f func() error) error {
	if t.rel.Load() {
		return tx.ErrDiscarded
	}
	err := f()
	if err == tx.ErrDiscarded {
		t.Discard()
	} else if err != nil {
		select {
		case <-t.ctx.Done():
			t.Discard()
		default:
		}
	}
	return err
}

// _ is a type assertion
var _ kvtx.Tx = ((*busObjectStoreTx)(nil))
