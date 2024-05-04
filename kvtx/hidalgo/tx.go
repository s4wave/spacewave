package kvtx_hidalgo

import (
	"bytes"
	"context"

	kv "github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/kv/options"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Tx implements the hidalgo kv t/x interface with a kvtx tx.
type Tx struct {
	tx kvtx.Tx
}

// NewTx constructs a new Tx.
func NewTx(tx kvtx.Tx) *Tx {
	return &Tx{
		tx: tx,
	}
}

// Get fetches a value for a single key from the database.
// It return ErrNotFound if key does not exists.
func (t *Tx) Get(ctx context.Context, key kv.Key) (kv.Value, error) {
	if len(key) == 0 {
		return nil, kv.ErrNotFound
	}
	data, found, err := t.tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, kv.ErrNotFound
	}
	return data, nil
}

// GetBatch fetches values for multiple keys from the database.
// Nil element in the slice indicates that key does not exists.
func (t *Tx) GetBatch(ctx context.Context, keys []kv.Key) ([]kv.Value, error) {
	var err error
	vals := make([]kv.Value, len(keys))
	for ki, k := range keys {
		if len(k) == 0 {
			continue
		}
		vals[ki], err = t.Get(ctx, k)
		if err != nil {
			if err == kv.ErrNotFound {
				vals[ki] = nil
			} else {
				return nil, err
			}
		}
	}
	return vals, nil
}

// Put writes a key-value pair to the database.
// New value will immediately be visible by Get on the same Tx,
// but implementation might buffer the write until transaction is committed.
func (t *Tx) Put(ctx context.Context, k kv.Key, v kv.Value) error {
	if len(k) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.tx.Set(ctx, k, v)
}

// Del removes the key from the database. See Put for consistency guaranties.
func (t *Tx) Del(ctx context.Context, k kv.Key) error {
	if len(k) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.tx.Delete(ctx, k)
}

// Scan will iterate over all key-value pairs.
// Expects them to arrive in order in the hidalgo kvtest.
// Use hidalgo/options.WithKVPrefix to specify a prefix for scanning.
func (t *Tx) Scan(ctx context.Context, opts ...kv.IteratorOption) kv.Iterator {
	iter := &txScanIterator{}
	var pref kv.Key
	for _, opt := range opts {
		pkv, ok := opt.(options.PrefixKV)
		if ok {
			pref = kv.KeyEscape(pkv.Pref)
		}
	}
	iter.err = t.tx.ScanPrefix(ctx, pref, func(key, value []byte) error {
		// Hidalgo expects them to arrive in order.
		// Unfortunately hydra does not guarantee this.
		// Perform a basic insertion sort.
		// TODO: is this necessary? it might be OK to iterate out of order.
		// TODO: ScanPrefix returns sorted values in most of our k/v stores.
		// TODO: implement a better iterator
		nv := &txScanIteratorValue{
			key:   key,
			value: value,
		}
		ov := iter.value
		nextPtr := &iter.value
		for ov != nil {
			// if the iterated value is greater than current, break
			if bytes.Compare(ov.key, key) > 0 {
				nv.next = ov
				break
			}
			// this node is less than kkey
			// set next ptr to this->next
			nextPtr = &ov.next
			ov = ov.next
		}

		*nextPtr = nv
		return nil
	})
	iter.first = true
	iter.start = iter.value
	iter.value = nil
	return iter
}

// Commit applies all changes made in the transaction.
func (t *Tx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Close rolls back the transaction.
// Committed transactions will not be affected by calling Close.
func (t *Tx) Close() error {
	t.tx.Discard()
	return nil
}

// _ is a type assertion
var _ kv.Tx = ((*Tx)(nil))
