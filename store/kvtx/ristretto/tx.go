package store_kvtx_ristretto

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/dgraph-io/ristretto"
)

// Tx implements a transaction on top of the cache.
// Note that ristretto does not support the tx semantics.
type Tx struct {
	rel atomic.Bool
	db  *ristretto.Cache
	ttl time.Duration
}

// NewTx constructs a new tx.
func NewTx(db *ristretto.Cache, ttl time.Duration) *Tx {
	return &Tx{db: db, ttl: ttl}
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	if t.rel.Load() {
		return 0, kvtx.ErrDiscarded
	}
	if t.db.Metrics == nil {
		return 0, errors.New("size not supported")
	}
	return t.db.Metrics.KeysAdded() - t.db.Metrics.KeysEvicted(), nil
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if t.rel.Load() {
		return nil, false, kvtx.ErrDiscarded
	}
	value, found := t.db.Get(key)
	if found {
		data, found = value.([]byte)
	}
	return data, found, nil
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if t.rel.Load() {
		return false, kvtx.ErrDiscarded
	}
	value, found := t.db.Get(key)
	if found {
		_, found = value.([]byte)
	}
	return found, nil
}

// Set sets the value of a key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if t.rel.Load() {
		return kvtx.ErrDiscarded
	}
	var ok bool
	cost := int64(len(value))
	if t.ttl != 0 {
		ok = t.db.SetWithTTL(key, value, cost, t.ttl)
	} else {
		ok = t.db.Set(key, value, cost)
	}
	// ignore if it was actually set or not
	_ = ok
	return nil
}

// Delete deletes a key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if t.rel.Load() {
		return kvtx.ErrDiscarded
	}
	t.db.Del(key)
	return nil
}

// ScanPrefix is not implemented with ristretto.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	if t.rel.Load() {
		return kvtx.ErrDiscarded
	}
	return errors.New("scan prefix is not supported")
}

// ScanPrefixKeys is not implemented with ristretto.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if t.rel.Load() {
		return kvtx.ErrDiscarded
	}
	return errors.New("scan prefix keys is not supported")
}

// Iterate is not implemented with ristretto.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	if t.rel.Load() {
		return kvtx.NewErrIterator(kvtx.ErrDiscarded)
	}
	return kvtx.NewErrIterator(errors.New("iterator is not supported"))
}

// Commit does nothing against ristretto
func (t *Tx) Commit(ctx context.Context) error {
	if t.rel.Swap(true) {
		return kvtx.ErrDiscarded
	}
	return nil
}

// Discard cancels the transaction (does nothing against ristretto).
func (t *Tx) Discard() {
	t.rel.Store(true)
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
