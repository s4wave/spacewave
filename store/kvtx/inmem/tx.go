package store_kvtx_inmem

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// Tx is a inmem transaction.
type Tx struct {
	s     *Store
	write bool

	// mtx guards below fields
	mtx sync.RWMutex
	// discarded indicates the tx has been discarded
	discarded atomic.Bool
	// added contains keys added
	added map[uint64]valType
	// deleted contains keys deleted
	deleted map[uint64]struct{}
}

// newTx constructs a new inmem transaction.
func newTx(s *Store, write bool) *Tx {
	tx := &Tx{
		s:     s,
		write: write,
	}
	if write {
		tx.added = map[uint64]valType{}
		tx.deleted = map[uint64]struct{}{}
	}
	return tx
}

// Get returns a value for a key.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	t.mtx.RLock()
	defer t.mtx.RUnlock()

	if t.discarded.Load() {
		return nil, false, kvtx.ErrDiscarded
	}

	keyHash := hashKey(key)
	var val valType
	var ok bool
	if t.write {
		if _, deleted := t.deleted[keyHash]; deleted {
			return nil, false, nil
		}
		val, ok = t.added[keyHash]
		if !ok {
			val, ok = t.s.m[keyHash]
		}
	} else {
		val, ok = t.s.m[keyHash]
	}
	if !ok {
		return nil, false, nil
	}
	out := make([]byte, len(val.val))
	copy(out, val.val)
	return out, true, nil
}

// Size returns the number of keys in the store.
func (t *Tx) Size() (uint64, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	if t.discarded.Load() {
		return 0, kvtx.ErrDiscarded
	}

	count := len(t.s.m)
	if t.write {
		count += len(t.added) - len(t.deleted)
	}
	return uint64(count), nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	keyHash := hashKey(key)
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	kb := make([]byte, len(key))
	copy(kb, key)
	vb := make([]byte, len(value))
	copy(vb, value)
	t.added[keyHash] = valType{
		key: kb,
		val: vb,
	}
	delete(t.deleted, keyHash)
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return errors.New("delete called on non-write tx")
	}
	keyHash := hashKey(key)
	t.mtx.Lock()
	if _, ok := t.s.m[keyHash]; ok {
		t.deleted[keyHash] = struct{}{}
	}
	delete(t.added, keyHash)
	t.mtx.Unlock()
	return nil
}

// ScanPrefix iterates over keys and values with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	t.mtx.RLock()
	if t.discarded.Load() {
		t.mtx.RUnlock()
		return kvtx.ErrDiscarded
	}

	var keys [][]byte
	enqueue := func(val valType) {
		if bytes.HasPrefix(val.key, prefix) {
			keys = append(keys, val.key)
		}
	}

	for keyHash, val := range t.s.m {
		if _, ok := t.deleted[keyHash]; ok {
			continue
		}
		if _, ok := t.added[keyHash]; ok {
			continue
		}
		enqueue(val)
	}
	for _, val := range t.added {
		enqueue(val)
	}
	t.mtx.RUnlock()

	for _, key := range keys {
		data, ok, err := t.Get(key)
		if err != nil {
			return err
		}
		if ok {
			if err := cb(key, data); err != nil {
				return err
			}
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

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	keyHash := hashKey(key)
	if _, ok := t.deleted[keyHash]; ok {
		return false, nil
	}
	if _, ok := t.added[keyHash]; ok {
		return true, nil
	}
	if _, ok := t.s.m[keyHash]; ok {
		return true, nil
	}
	return false, nil
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	if !t.write {
		t.Discard()
		return kvtx.ErrNotWrite
	}
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.discarded.Swap(true) {
		return kvtx.ErrDiscarded
	}
	for key, val := range t.added {
		t.s.m[key] = val
	}
	t.added = nil
	for key := range t.deleted {
		delete(t.s.m, key)
	}
	t.deleted = nil
	t.s.writing = false
	t.s.bcast.Broadcast()
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.discarded.Swap(true) {
		return
	}
	t.added, t.deleted = nil, nil
	if t.write {
		t.s.writing = false
	} else {
		t.s.nreaders--
	}
	t.s.bcast.Broadcast()
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
