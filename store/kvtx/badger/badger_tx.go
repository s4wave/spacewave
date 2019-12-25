package store_kvtx_badger

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	bdb "github.com/dgraph-io/badger/v2"
)

// Tx is a badger transaction.
type Tx struct {
	s          *Store
	txn        *bdb.Txn
	commitOnce sync.Once
	write      bool
}

// NewTx constructs a new badger transaction.
func (s *Store) newTx(txn *bdb.Txn, write bool) *Tx {
	return &Tx{s: s, txn: txn, write: write}
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

	return valb, true, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	_ = ttl // TODO
	return t.txn.Set(key, value)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	it := t.txn.NewIterator(bdb.DefaultIteratorOptions)
	defer it.Close()

	valid := it.Valid
	if len(prefix) == 0 {
		it.Rewind()
	} else {
		it.Seek(prefix)
		valid = func() bool {
			return it.ValidForPrefix(prefix)
		}
	}

	for valid() {
		item := it.Item()
		k := item.Key()
		if err := item.Value(func(val []byte) error {
			return cb(k, val)
		}); err != nil {
			return err
		}
		it.Next()
	}
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	return t.txn.Delete(key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	var err error
	t.commitOnce.Do(func() {
		err = t.txn.Commit()
		if t.write {
			t.s.writeMtx.Unlock()
		}
	})
	return err
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	i, err := t.txn.Get(key)
	if err != nil {
		if err == bdb.ErrKeyNotFound {
			return false, nil
		}
		return false, err
	}
	return i != nil, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.commitOnce.Do(func() {
		if t.write {
			t.s.writeMtx.Unlock()
		}
	})
	t.txn.Discard()
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
