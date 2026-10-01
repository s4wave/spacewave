package store_kvtx_inmem

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Tx holds a store snapshot and the changes staged by an exclusive writer.
type Tx struct {
	// s retains the store whose admission this transaction holds.
	s *Store
	// write permits staging changes and excludes other transactions.
	write bool

	// discarded prevents Commit or Discard from releasing admission twice.
	discarded atomic.Bool
	// added contains inserted and replaced values, absent from deleted.
	added *btree.BTreeG[*valType]
	// deleted contains committed keys removed by this transaction.
	deleted *btree.BTreeG[*valType]
}

// newTx constructs a new inmem transaction.
func newTx(s *Store, write bool) *Tx {
	// Retain the admission granted by the store.
	tx := &Tx{
		s:     s,
		write: write,
	}

	// Only a writer needs overlays for tentative changes.
	if write {
		tx.added = btree.NewBTreeG(valTypeLess)
		tx.deleted = btree.NewBTreeG(valTypeLess)
	}
	return tx
}

// Get returns a value for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Reject an invalid lookup before reading the transaction's snapshot.
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if t.discarded.Load() {
		return nil, false, kvtx.ErrDiscarded
	}

	// Staged deletion and replacement take precedence over the stored value.
	item := &valType{key: key}
	if t.write {
		if _, exists := t.deleted.Get(item); exists {
			return nil, false, nil
		}
		if val, exists := t.added.Get(item); exists {
			return bytes.Clone(val.val), true, nil
		}
	}

	// Copy the committed value so the caller cannot mutate the snapshot.
	val, exists := t.s.tree.Get(item)
	if !exists {
		return nil, false, nil
	}
	return bytes.Clone(val.val), true, nil
}

// Size returns the number of keys visible in the transaction.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	// A discarded transaction no longer owns a readable snapshot.
	if t.discarded.Load() {
		return 0, kvtx.ErrDiscarded
	}

	// Readers see the committed tree's cardinality directly.
	count := t.s.tree.Len()
	if !t.write {
		return uint64(count), nil //nolint:gosec // A tree length is nonnegative.
	}

	// Replacements already belong to the tree; count only newly inserted keys.
	count -= t.deleted.Len()
	t.added.Ascend(nil, func(item *valType) bool {
		if _, exists := t.s.tree.Get(item); !exists {
			count++
		}
		return true
	})
	return uint64(count), nil //nolint:gosec // Deletions are a subset of committed keys.
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	// Require a live writer and a nonempty key before staging a value.
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	// Own the replacement bytes and supersede any staged deletion.
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
	// Require a live writer and a nonempty key before staging a deletion.
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	// Delete only committed keys; a tentative insertion can simply disappear.
	item := &valType{key: key}
	if _, valExists := t.s.tree.Get(item); valExists {
		t.deleted.Set(item)
	}
	t.added.Delete(item)
	return nil
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	// A discarded transaction no longer owns keys to scan.
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	// Collect visible committed keys before invoking callbacks that may mutate.
	// TODO: Merge committed and staged ranges without copying all visible keys.
	var keys [][]byte
	var pivot *valType
	if len(prefix) != 0 {
		pivot = &valType{key: prefix}
	}
	t.s.tree.Ascend(pivot, func(item *valType) bool {
		// Stop when the committed range leaves the requested prefix.
		if !bytes.HasPrefix(item.key, prefix) {
			return false
		}

		// Retain only committed keys that are not shadowed by an overlay.
		if !t.write {
			keys = append(keys, item.key)
			return true
		}
		if _, exists := t.deleted.Get(item); exists {
			return true
		}
		if _, exists := t.added.Get(item); !exists {
			keys = append(keys, item.key)
		}
		return true
	})

	// Include staged insertions and replacements once each.
	if t.write {
		t.added.Ascend(pivot, func(item *valType) bool {
			if bytes.HasPrefix(item.key, prefix) {
				keys = append(keys, item.key)
			}
			return true
		})
	}

	// Let callbacks stop the scan with their own error.
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
	// Reject an invalid lookup before reading the transaction's snapshot.
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	if t.discarded.Load() {
		return false, kvtx.ErrDiscarded
	}

	// Staged deletion and replacement take precedence over the stored key.
	item := &valType{key: key}
	if t.write {
		if _, valExists := t.deleted.Get(item); valExists {
			return false, nil
		}
		if _, valExists := t.added.Get(item); valExists {
			return true, nil
		}
	}

	// Fall back to the committed snapshot.
	_, exists := t.s.tree.Get(item)
	return exists, nil
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Returns an error if called after Discard.
func (t *Tx) Commit(ctx context.Context) error {
	// Finish the transaction once, rejecting a read-only commit.
	if !t.write {
		t.Discard()
		return kvtx.ErrNotWrite
	}
	wasDiscarded := t.discarded.Swap(true)
	if wasDiscarded {
		return kvtx.ErrDiscarded
	}

	// Publish both overlays before admitting the next reader or writer.
	t.s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Replace committed values with the staged insertions and updates.
		t.added.Ascend(nil, func(item *valType) bool {
			t.s.tree.Set(item)
			return true
		})
		t.added = nil

		// Apply deletions before releasing exclusive access.
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
	// Drop tentative changes only once, including after a completed commit.
	wasDiscarded := t.discarded.Swap(true)
	if wasDiscarded {
		return
	}
	t.added, t.deleted = nil, nil

	// Release this transaction's admission and wake the remaining waiters.
	t.s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		switch t.write {
		case true:
			t.s.writing = false
		case false:
			t.s.nreaders--
		}
		broadcast()
	})
}

// _ checks the transaction interface.
var _ kvtx.Tx = (*Tx)(nil)
