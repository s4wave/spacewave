package store_kvtx_inmem

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// TODO: it should be possible to construct one iterator for tree & one for added, then increment them together.
// TODO: this would eliminate the requirement to load all the keys into a separate slice first.

// Tx is a inmem transaction.
type Tx struct {
	s     *Store
	write bool

	// discarded indicates the tx has been discarded
	discarded atomic.Bool
	// added contains items to be added
	added *btree.BTreeG[*valType]
	// deleted contains keys to be deleted
	deleted *btree.BTreeG[*valType]
}

// newTx constructs a new inmem transaction.
func newTx(s *Store, write bool) *Tx {
	tx := &Tx{
		s:     s,
		write: write,
	}
	if write {
		tx.added = btree.NewBTreeG(valTypeLess)
		tx.deleted = btree.NewBTreeG(valTypeLess)
	}
	return tx
}

// Get returns a value for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	if t.discarded.Load() {
		return nil, false, kvtx.ErrDiscarded
	}

	searchItem := &valType{key: key}
	var val *valType
	var valExists bool
	if t.write {
		// if the value was deleted, return early.
		if _, valExists = t.deleted.Get(searchItem); valExists {
			return nil, false, nil
		}
		// if the value was added, use that value.
		if val, valExists = t.added.Get(searchItem); !valExists {
			// otherwise fetch from the tree
			val, valExists = t.s.tree.Get(searchItem)
		}
	} else {
		// if read-only read directly from the tree
		val, valExists = t.s.tree.Get(searchItem)
	}
	if !valExists || val == nil {
		return nil, false, nil
	}
	return bytes.Clone(val.val), true, nil
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	if t.discarded.Load() {
		return 0, kvtx.ErrDiscarded
	}

	count := t.s.tree.Len()
	if t.write {
		count += t.added.Len() - t.deleted.Len()
	}
	return uint64(count), nil //nolint:gosec
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	kb, vb := bytes.Clone(key), bytes.Clone(value)
	item := &valType{key: kb, val: vb}
	t.added.Set(item)
	t.deleted.Delete(item)
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	item := &valType{key: key}
	if _, valExists := t.s.tree.Get(item); valExists {
		t.deleted.Set(item)
	}
	t.added.Delete(item)
	return nil
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	var keys [][]byte
	var pivot *valType
	if len(prefix) != 0 {
		pivot = &valType{key: prefix}
	}
	t.s.tree.Ascend(pivot, func(item *valType) bool {
		if !bytes.HasPrefix(item.key, prefix) {
			return false
		}
		if t.write {
			if _, delExists := t.deleted.Get(item); !delExists {
				if _, addExists := t.added.Get(item); !addExists {
					keys = append(keys, item.key)
				}
			}
		} else {
			keys = append(keys, item.key)
		}
		return true
	})
	if t.write {
		t.added.Ascend(pivot, func(item *valType) bool {
			if bytes.HasPrefix(item.key, prefix) {
				keys = append(keys, item.key)
			}
			return true
		})
	}

	for _, key := range keys {
		if err := cb(key); err != nil {
			return err
		}
	}

	return nil
}

// ScanPrefix iterates over keys and values with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		data, ok, err := t.Get(ctx, key)
		if err != nil {
			return err
		}
		if ok {
			return cb(key, data)
		}
		return nil
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	if t.discarded.Load() {
		return false, kvtx.ErrDiscarded
	}
	item := &valType{key: key}
	if t.write {
		if _, valExists := t.deleted.Get(item); valExists {
			return false, nil
		}
		if _, valExists := t.added.Get(item); valExists {
			return true, nil
		}
	}
	if _, valExists := t.s.tree.Get(item); valExists {
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
	wasDiscarded := t.discarded.Swap(true)
	if wasDiscarded {
		return kvtx.ErrDiscarded
	}

	t.s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		t.added.Ascend(nil, func(item *valType) bool {
			t.s.tree.Set(item)
			return true
		})
		t.added = nil
		t.deleted.Ascend(nil, func(item *valType) bool {
			t.s.tree.Delete(item)
			return true
		})
		t.deleted = nil
		t.s.writing = false
		broadcast()
	})
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	wasDiscarded := t.discarded.Swap(true)
	if wasDiscarded {
		return
	}
	t.added, t.deleted = nil, nil

	t.s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if t.write {
			t.s.writing = false
		} else {
			t.s.nreaders--
		}
		broadcast()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
