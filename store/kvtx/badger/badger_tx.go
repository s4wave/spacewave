package badger

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/store/kvtx"
	bdb "github.com/dgraph-io/badger"
)

// Tx is a badger transaction.
type Tx struct {
	txn *bdb.Txn
}

// NewTx constructs a new badger transaction.
func NewTx(txn *bdb.Txn) *Tx {
	return &Tx{txn: txn}
}

// Get returns values for one or more keys.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	item, err := t.txn.Get(key)
	if err != nil {
		if err == bdb.ErrKeyNotFound {
			err = nil
		}
		return nil, false, err
	}

	var valb []byte
	err = item.Value(func(val []byte) error {
		valb = make([]byte, len(val))
		copy(valb, val)
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	return valb, false, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	if ttl == time.Duration(0) {
		return t.txn.Set(key, value)
	}

	return t.txn.SetWithTTL(key, value, ttl)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	return t.txn.Commit()
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.txn.Discard()
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
