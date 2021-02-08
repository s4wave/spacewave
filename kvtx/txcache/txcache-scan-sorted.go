package kvtx_txcache

import (
	"bytes"
	"context"
	"slices"

	"github.com/tidwall/btree"
)

// scanPrefixSorted implements ScanPrefix sorted.
func (t *TXCache) scanPrefixSorted(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	type scanVal struct {
		key   []byte
		value []byte
	}
	var vals []scanVal
	err := t.underlying.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		searchItem := &cacheItem{key: key}
		if _, removed := t.remove.Get(searchItem); removed {
			return nil
		}
		if _, overridden := t.set.Get(searchItem); overridden {
			return nil
		}
		vals = append(vals, scanVal{
			key:   bytes.Clone(key),
			value: bytes.Clone(value),
		})
		return nil
	})
	if err != nil {
		return err
	}
	t.set.Ascend(nil, func(item *cacheItem) bool {
		searchItem := &cacheItem{key: item.key}
		if _, removed := t.remove.Get(searchItem); removed {
			return true
		}
		vals = append(vals, scanVal{
			key:   item.key,
			value: item.val,
		})
		return true
	})
	slices.SortFunc(vals, func(a, b scanVal) int {
		return bytes.Compare(a.key, b.key)
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
func (t *TXCache) scanPrefixUnsorted(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	t.mtx.RLock()
	snapRemove := t.remove
	snapSet := t.set
	t.mtx.RUnlock()
	seen := btree.NewBTreeG[*cacheItem](func(a, b *cacheItem) bool { return a.Less(b) })

	err := t.underlying.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		searchItem := &cacheItem{key: key}
		if _, removed := snapRemove.Get(searchItem); removed {
			return nil
		}
		if item, overridden := snapSet.Get(searchItem); overridden {
			seen.Set(&cacheItem{key: key})
			return cb(key, item.val)
		}
		return cb(key, value)
	})
	if err != nil {
		return err
	}

	snapSet.Ascend(nil, func(item *cacheItem) bool {
		searchItem := &cacheItem{key: item.key}
		if _, ok := snapRemove.Get(searchItem); ok {
			return true
		}
		if _, ok := seen.Get(searchItem); ok {
			return true
		}
		if err = cb(item.key, item.val); err != nil {
			return false
		}
		return true
	})
	return nil
}
