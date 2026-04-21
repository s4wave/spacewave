package kvtx_txcache

import (
	"bytes"
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_iterator "github.com/s4wave/spacewave/db/kvtx/iterator"
	"github.com/tidwall/btree"
)

// TODO: check for concurrent access issues

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
	set        *btree.BTreeG[*cacheItem]
	remove     *btree.BTreeG[*cacheItem]
}

// NewTXCache implements the transaction cache in-memory.
//
// if sortScan is set, ScanPrefix results will be sorted (costs memory)
// cacheItem represents an item in the cache
type cacheItem struct {
	key []byte
	val []byte
}

// cacheItemLess implements the less function for cacheItem comparison
// may be called with nil if we pass nil for the pivot
func cacheItemLess(a, b *cacheItem) bool {
	var aKey, bKey []byte
	if a != nil {
		aKey = a.key
	}
	if b != nil {
		bKey = b.key
	}
	return bytes.Compare(aKey, bKey) < 0
}

// Less implements btree.Item interface
func (c *cacheItem) Less(than *cacheItem) bool {
	return cacheItemLess(c, than)
}

func NewTXCache(underlying kvtx.TxOps, sortScan bool) *TXCache {
	return &TXCache{
		underlying: underlying,
		set:        btree.NewBTreeG[*cacheItem](cacheItemLess),
		remove:     btree.NewBTreeG[*cacheItem](cacheItemLess),
		sortScan:   sortScan,
	}
}

// checkWasRemoved checks if the key was removed
func checkWasRemoved(tree *btree.BTreeG[*cacheItem], key []byte) bool {
	searchItem := &cacheItem{key: key}
	_, ok := tree.Get(searchItem)
	return ok
}

// checkWasAdded checks if the key was added
func checkWasAdded(tree *btree.BTreeG[*cacheItem], key []byte) ([]byte, bool) {
	searchItem := &cacheItem{key: key}
	item, ok := tree.Get(searchItem)
	if !ok {
		return nil, false
	}
	return item.val, true
}

// WasAdded checks if the key is in the added map.
func (t *TXCache) WasAdded(key []byte) ([]byte, bool) {
	if checkWasRemoved(t.remove, key) {
		return nil, false
	}
	return checkWasAdded(t.set, key)
}

// WasRemoved checks if the key is in the tombstone map.
func (t *TXCache) WasRemoved(key []byte) bool {
	searchItem := &cacheItem{key: key}
	_, ok := t.remove.Get(searchItem)
	return ok
}

// Get returns values for a key.
func (t *TXCache) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	t.mtx.RLock()
	snapRemove := t.remove
	snapSet := t.set
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
	removeN := t.remove.Len()
	setN := t.set.Len()
	underlyingN, err := t.underlying.Size(ctx)
	t.mtx.RUnlock()
	if err != nil {
		return 0, err
	}
	return underlyingN + uint64(setN) - uint64(removeN), nil //nolint:gosec
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *TXCache) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	t.mtx.Lock()
	searchItem := &cacheItem{key: key}
	t.remove.Delete(searchItem)
	t.set.Set(&cacheItem{
		key: bytes.Clone(key),
		val: bytes.Clone(value),
	})
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
	searchItem := &cacheItem{key: key}
	t.set.Delete(searchItem)
	t.remove.Set(&cacheItem{key: key})
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
	snapRemove := t.remove
	snapSet := t.set
	t.mtx.RUnlock()

	searchItem := &cacheItem{key: key}
	if _, ok := snapRemove.Get(searchItem); ok {
		return false, nil
	}
	if _, ok := snapSet.Get(searchItem); ok {
		return true, nil
	}
	return t.underlying.Exists(ctx, key)
}

// _ is a type assertion
var _ kvtx.TxOps = ((*TXCache)(nil))
