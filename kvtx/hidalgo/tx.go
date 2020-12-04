package kvtx_hidalgo

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	kv "github.com/hidal-go/hidalgo/kv/flat"
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
	data, found, err := t.tx.Get(key)
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
func (t *Tx) Put(k kv.Key, v kv.Value) error {
	return t.tx.Set(k, v, time.Duration(0))
}

// Del removes the key from the database. See Put for consistency guaranties.
func (t *Tx) Del(k kv.Key) error {
	return t.tx.Delete(k)
}

// Scan will iterate over all key-value pairs with a specific key prefix.
// Expects them to arrive in order in the hidalgo kvtest.
func (t *Tx) Scan(pref kv.Key) kv.Iterator {
	iter := &txScanIterator{}
	t.tx.ScanPrefix(pref, func(key, value []byte) error {
		// Hidalgo expects them to arrive in order.
		// Unfortunately hydra does not guarantee this.
		// Perform a basic insertion sort.
		// TODO see implementation in kvcache
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
	iter.first = iter.value != nil
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
