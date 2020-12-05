package store_kvtx_inmem

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Workiva/go-datastructures/trie/ctrie"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// Tx is a inmem transaction.
type Tx struct {
	s           *Store
	discardOnce sync.Once
	write       bool
	ct          *ctrie.Ctrie
}

// newTx constructs a new inmem transaction.
func newTx(s *Store, write bool, ct *ctrie.Ctrie) *Tx {
	return &Tx{s: s, write: write, ct: ct}
}

// Get returns a value for a key.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	di, diOk := t.ct.Lookup(key)
	if !diOk {
		return nil, false, nil
	}
	dib := di.([]byte)
	dic := make([]byte, len(dib))
	copy(dic, dib)
	return dic, true, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	if !t.write {
		return errors.New("set called on non-write tx")
	}
	if ttl != 0 {
		// TODO
		return errors.New("ttl not implemented in in-mem store")
	}
	vb := make([]byte, len(value))
	copy(vb, value)
	t.ct.Insert(key, vb)
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	if !t.write {
		return errors.New("delete called on non-write tx")
	}
	_, _ = t.ct.Remove(key)
	return nil
}

// ScanPrefix iterates over keys and values with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	cancel := make(chan struct{})
	defer close(cancel)

	ch := t.ct.Iterator(cancel)
	for val := range ch {
		k := val.Key
		if len(prefix) != 0 && !bytes.HasPrefix(k, prefix) {
			continue
		}
		if err := cb(k, val.Value.([]byte)); err != nil {
			return err
		}
	}
	return nil
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
func (t *Tx) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(t, prefix, sort, reverse)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	if !t.write {
		t.Discard()
		return errors.New("commit called on non-write tx")
	}

	t.discardOnce.Do(func() {
		t.s.mtx.Lock()
		t.s.ct = t.ct
		t.s.mtx.Unlock()
		// locked when creating tx
		t.s.writeMtx.Unlock()
	})
	return nil
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	_, ok := t.ct.Lookup(key)
	return ok, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.discardOnce.Do(func() {
		if t.write {
			t.s.writeMtx.Unlock()
		}
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
