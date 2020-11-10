package kvtx_txcache

import (
	"bytes"
	"sort"
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
