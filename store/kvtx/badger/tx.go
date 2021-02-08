package store_kvtx_badger

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
	bdb "github.com/dgraph-io/badger/v4"
)

// Tx is a badger transaction.
type Tx struct {
	s          *Store
	txn        *bdb.Txn
	commitOnce sync.Once
	write      bool
	mtx        sync.Mutex
	rel        bool
	iters      map[*Iterator]struct{}
}

// NewTx constructs a new badger transaction.
func (s *Store) newTx(txn *bdb.Txn, write bool) *Tx {
	return &Tx{s: s, txn: txn, write: write}
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
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

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	return 0, kvtx.ErrBlockTxOpsUnimplemented
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.txn.Set(key, value)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
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

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
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
		if err := cb(k); err != nil {
			return err
		}
		it.Next()
	}

	return nil
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse MAY have no effect.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	opts := bdb.DefaultIteratorOptions
	opts.Reverse = reverse
	opts.Prefix = prefix
	opts.AllVersions = false

	t.mtx.Lock()
	rel := t.rel
	var it *Iterator
	if !rel {
		it = NewIterator(t.txn.NewIterator(opts), reverse, prefix, func() {
			t.mtx.Lock()
			if it != nil && t.iters != nil {
				it = nil
				delete(t.iters, it)
			}
			t.mtx.Unlock()
		})
		if t.iters == nil {
			t.iters = make(map[*Iterator]struct{})
		}
		t.iters[it] = struct{}{}
	}
	t.mtx.Unlock()

	return it
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.txn.Delete(key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	var err error
	t.commitOnce.Do(func() {
		t.mtx.Lock()
		t.rel = true
		// ensure all iterators are closed
		for it := range t.iters {
			it.it.Close()
		}
		t.iters = nil
		t.mtx.Unlock()
		err = t.txn.Commit()
		if t.write {
			t.s.writeMtx.Unlock()
		}
	})
	return err
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
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
		t.mtx.Lock()
		t.rel = true
		// ensure all iterators are closed
		for it := range t.iters {
			it.it.Close()
		}
		t.iters = nil
		t.mtx.Unlock()
		if t.write {
			t.s.writeMtx.Unlock()
		}
	})
	t.txn.Discard()
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
