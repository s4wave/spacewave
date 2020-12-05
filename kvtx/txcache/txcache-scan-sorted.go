package kvtx_txcache

import (
	"bytes"
	"sort"

	"github.com/Workiva/go-datastructures/trie/ctrie"
)

// scanPrefixSorted implements ScanPrefix sorted.
func (t *TXCache) scanPrefixSorted(prefix []byte, cb func(key, value []byte) error) error {
	t.mtx.RLock()
	snapRemove := t.remove.ReadOnlySnapshot()
	snapSet := t.set.ReadOnlySnapshot()
	t.mtx.RUnlock()

	type scanVal struct {
		key   []byte
		value []byte
	}
	var vals []scanVal
	err := t.underlying.ScanPrefix(prefix, func(key, value []byte) error {
		if _, removed := snapRemove.Lookup(key); removed {
			return nil
		}
		if _, overridden := snapSet.Lookup(key); overridden {
			return nil
		}
		kc := make([]byte, len(key))
		copy(kc, key)
		kv := make([]byte, len(value))
		copy(kv, value)
		vals = append(vals, scanVal{
			key:   kc,
			value: kv,
		})
		return nil
	})
	if err != nil {
		return err
	}
	setIter := snapSet.Iterator(nil)
	for added := range setIter {
		if _, removed := snapRemove.Lookup(added.Key); removed {
			// possibly unnecessary - double-check to be sure.
			continue
		}
		vals = append(vals, scanVal{
			key:   added.Key,
			value: added.Value.([]byte),
		})
	}
	sort.Slice(vals, func(i int, j int) bool {
		return bytes.Compare(vals[i].key, vals[j].key) == -1
	})
	for i, val := range vals {
		if err := cb(val.key, val.value); err != nil {
			return err
		}
		// release memory
		vals[i] = scanVal{}
	}
	return nil
}

// scanPrefixUnsorted implements ScanPrefix unsorted.
func (t *TXCache) scanPrefixUnsorted(prefix []byte, cb func(key, value []byte) error) error {
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
