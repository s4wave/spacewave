package kvtx

import (
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/hydra/tx"
)

// TxStoreTx implements the Tx interface backed by a TxOps
// See TxStore.
type TxStoreTx struct {
	// tx is the underlying tx
	tx TxOps
	// rmtx guards discarded field
	rmtx sync.RWMutex
	// discarded indicates the tx was already discarded
	discarded bool
}

// NewTxStoreTx constructs a new TxStoreTx from a TxOps
func NewTxStoreTx(ops TxOps) (*TxStoreTx, error) {
	if ops == nil {
		return nil, errors.New("tx ops cannot be empty")
	}
	return &TxStoreTx{tx: ops}, nil
}

// GetTxOps returns the transaction ops object.
func (t *TxStoreTx) GetTxOps() TxOps {
	return t.tx
}

// Get returns values for a key.
func (t *TxStoreTx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if len(key) == 0 {
		return nil, false, ErrEmptyKey
	}
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return nil, false, tx.ErrDiscarded
	}

	return t.tx.Get(ctx, key)
}

// Size returns the number of keys in the store.
func (t *TxStoreTx) Size(ctx context.Context) (uint64, error) {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return 0, tx.ErrDiscarded
	}

	return t.tx.Size(ctx)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *TxStoreTx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return ErrEmptyKey
	}
	// note: we don't write discarded field, so use RLock
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.tx.Set(ctx, key, value)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *TxStoreTx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return ErrEmptyKey
	}
	// note: we don't write discarded field, so use RLock
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.tx.Delete(ctx, key)
}

// ScanPrefix iterates over keys with a prefix.
//
// Note: neither key nor value should be retained outside cb() without
// copying.
//
// Note: the ordering of the scan is not necessarily sorted.
func (t *TxStoreTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.tx.ScanPrefix(ctx, prefix, cb)
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *TxStoreTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.tx.ScanPrefixKeys(ctx, prefix, cb)
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
// Some implementations return BlockIterator.
func (t *TxStoreTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) Iterator {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return NewErrIterator(tx.ErrDiscarded)
	}

	return t.tx.Iterate(ctx, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *TxStoreTx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, ErrEmptyKey
	}
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return false, tx.ErrDiscarded
	}

	return t.tx.Exists(ctx, key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// TxStore: does nothing internally
// TxStore: if called after Discard, returns ErrDiscarded
// TxStore: all ops will return ErrDiscarded if called after Commit or Discard
func (t *TxStoreTx) Commit(ctx context.Context) error {
	return t.discardOnce()
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *TxStoreTx) Discard() {
	_ = t.discardOnce()
}

// discardOnce locks & discards the tx, returns ErrDiscarded if already discarded
func (t *TxStoreTx) discardOnce() error {
	t.rmtx.Lock()
	discarded := t.discarded
	if !discarded {
		t.discarded = true
	}
	t.rmtx.Unlock()

	if discarded {
		return tx.ErrDiscarded
	}
	return nil
}

// _ is a type assertion
var _ Tx = ((*TxStoreTx)(nil))
