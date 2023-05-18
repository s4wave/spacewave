package kvtx_txcache

import (
	"context"
	"sync"

	"github.com/Workiva/go-datastructures/trie/ctrie"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// TXCache overlays an in-memory map over a kvtx transaction to buffer changes
// for a transaction. Used for databases that do not support transactions, to
// buffer the changes in-memory until the tx Commit() is called.
//
// Call BuildOps() to return a sorted list of operations (add/remove). Discard the
// TXCache to clear it / reset it.
type TXCache struct {
	mtx        sync.RWMutex
	underlying kvtx.TxOps
	sortScan   bool
	set        *ctrie.Ctrie
	remove     *ctrie.Ctrie
}

// NewTXCache implements the transaction cache in-memory.
//
// if sortScan is set, ScanPrefix results will be sorted (costs memory)
func NewTXCache(underlying kvtx.TxOps, sortScan bool) *TXCache {
	return &TXCache{
		underlying: underlying,
		set:        ctrie.New(nil),
		remove:     ctrie.New(nil),
		sortScan:   sortScan,
	}
}

// checkWasRemoved checks if the key was removed
func checkWasRemoved(snapRemove *ctrie.Ctrie, key []byte) bool {
	_, removed := snapRemove.Lookup(key)
	return removed
}

// checkWasAdded checks if the key was added
func checkWasAdded(snapSet *ctrie.Ctrie, key []byte) ([]byte, bool) {
	v, ok := snapSet.Lookup(key)
	if !ok {
		return nil, false
	}
	return v.([]byte), true
}

// WasAdded checks if the key is in the added map.
func (t *TXCache) WasAdded(key []byte) ([]byte, bool) {
	t.mtx.RLock()
	snapRemove := t.remove.ReadOnlySnapshot()
	snapSet := t.set.ReadOnlySnapshot()
	t.mtx.RUnlock()
	if checkWasRemoved(snapRemove, key) {
		return nil, false
	}
	return checkWasAdded(snapSet, key)
}

// WasRemoved checks if the key is in the tombstone map.
func (t *TXCache) WasRemoved(key []byte) bool {
	t.mtx.RLock()
	snap := t.remove.ReadOnlySnapshot()
	t.mtx.RUnlock()
	_, ok := snap.Lookup(key)
	return ok
}

// Get returns values for a key.
func (t *TXCache) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	t.mtx.RLock()
	snapRemove := t.remove.ReadOnlySnapshot()
	snapSet := t.set.ReadOnlySnapshot()
	t.mtx.RUnlock()

	if checkWasRemoved(snapRemove, key) {
		return nil, false, nil
	}
	if val, ok := checkWasAdded(snapSet, key); ok {
		return val, true, nil
	}
	return t.underlying.Get(ctx, key)
}

// Size returns the number of keys in the store plus the added keys from the tx.
func (t *TXCache) Size(ctx context.Context) (uint64, error) {
	t.mtx.RLock()
	removeN := t.remove.Size()
	setN := t.set.Size()
	underlyingN, err := t.underlying.Size(ctx)
	t.mtx.RUnlock()
	if err != nil {
		return 0, err
	}
	return underlyingN + uint64(setN) - uint64(removeN), nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *TXCache) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	t.mtx.Lock()
	_, _ = t.remove.Remove(key)
	t.set.Insert(key, value)
	t.mtx.Unlock()
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *TXCache) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	t.mtx.Lock()
	_, _ = t.set.Remove(key)
	t.remove.Insert(key, nil)
	t.mtx.Unlock()
	return nil
}

// ScanPrefix iterates over keys with a prefix.
//
// To enforce ordering, it builds a set in memory, sorts, then operates.
func (t *TXCache) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	if t.sortScan {
		return t.scanPrefixSorted(ctx, prefix, cb)
	}
	return t.scanPrefixUnsorted(ctx, prefix, cb)
}

// ScanPrefixKeys iterates over keys with a prefix.
//
// To enforce ordering, it builds a set in memory, sorts, then operates.
func (t *TXCache) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// Iterates in sorted order, reverse reverses the key iteration.
func (t *TXCache) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(ctx, newIterOps(t), prefix, sort, reverse)
}

// Exists checks if a key exists.
func (t *TXCache) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	t.mtx.RLock()
	snapRemove := t.remove.ReadOnlySnapshot()
	snapSet := t.set.ReadOnlySnapshot()
	t.mtx.RUnlock()

	if _, ok := snapRemove.Lookup(key); ok {
		return false, nil
	}
	if _, ok := snapSet.Lookup(key); ok {
		return true, nil
	}
	return t.underlying.Exists(ctx, key)
}

// _ is a type assertion
var _ kvtx.TxOps = ((*TXCache)(nil))
