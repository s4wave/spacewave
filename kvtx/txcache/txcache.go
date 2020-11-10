package kvtx_txcache

import (
	"sync"
	"time"

	"github.com/Workiva/go-datastructures/trie/ctrie"
	"github.com/aperturerobotics/hydra/kvtx"
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
	ttl        *ctrie.Ctrie
}

// NewTXCache implements the transaction cache in-memory.
//
// if sortScan is set, ScanPrefix results will be sorted (costs memory)
func NewTXCache(underlying kvtx.TxOps, sortScan bool) *TXCache {
	return &TXCache{
		underlying: underlying,
		set:        ctrie.New(nil),
		remove:     ctrie.New(nil),
		ttl:        ctrie.New(nil),
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
func (t *TXCache) Get(key []byte) (data []byte, found bool, err error) {
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
	return t.underlying.Get(key)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *TXCache) Set(key, value []byte, ttl time.Duration) error {
	t.mtx.Lock()
	_, _ = t.remove.Remove(key)
	t.set.Insert(key, value)
	t.ttl.Insert(key, ttl)
	t.mtx.Unlock()
	return nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *TXCache) Delete(key []byte) error {
	t.mtx.Lock()
	_, _ = t.set.Remove(key)
	t.remove.Insert(key, nil)
	t.mtx.Unlock()
	return nil
}

// ScanPrefix iterates over keys with a prefix.
//
// To enforce ordering, it builds a set in memory, sorts, then operates.
func (t *TXCache) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	if t.sortScan {
		return t.scanPrefixSorted(prefix, cb)
	}

	t.mtx.RLock()
	snapRemove := t.remove.ReadOnlySnapshot()
	snapSet := t.set.ReadOnlySnapshot()
	t.mtx.RUnlock()
	seen := ctrie.New(nil)

	err := t.underlying.ScanPrefix(prefix, func(key, value []byte) error {
		if _, removed := snapRemove.Lookup(key); removed {
			return nil
		}
		if ov, overridden := snapSet.Lookup(key); overridden {
			seen.Insert(key, nil)
			return cb(key, ov.([]byte))
		}
		return cb(key, value)
	})
	if err != nil {
		return err
	}

	setIter := snapSet.Iterator(nil)
	for added := range setIter {
		if _, ok := snapRemove.Lookup(added.Key); ok {
			continue
		}
		if _, ok := seen.Lookup(added.Key); ok {
			continue
		}
		if err := cb(added.Key, added.Value.([]byte)); err != nil {
			return err
		}
	}
	return nil
}

// Exists checks if a key exists.
func (t *TXCache) Exists(key []byte) (bool, error) {
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
	return t.underlying.Exists(key)
}

// _ is a type assertion
var _ kvtx.TxOps = ((*TXCache)(nil))
