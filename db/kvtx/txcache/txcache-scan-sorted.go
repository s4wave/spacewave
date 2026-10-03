package kvtx_txcache

import (
	"bytes"
	"context"
	"slices"

	"github.com/tidwall/btree"
)

// scanPrefixSorted implements ScanPrefix sorted.
func (t *TXCache) scanPrefixSorted(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	// Collect underlying prefix values that the transaction cache leaves visible.
	type scanVal struct {
		key   []byte
		value []byte
	}
	var vals []scanVal
	err := t.underlying.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Exclude underlying keys deleted or replaced by pending cache changes.
		searchItem := &cacheItem{key: key}
		if _, removed := t.remove.Get(searchItem); removed {
			return nil
		}
		if _, overridden := t.set.Get(searchItem); overridden {
			return nil
		}

		// Retain copies of the visible underlying key and value for sorted delivery.
		vals = append(vals, scanVal{
			key:   bytes.Clone(key),
			value: bytes.Clone(value),
		})
		return nil
	})
	if err != nil {
		return err
	}

	// Collect pending cache values within the requested prefix.
	t.set.Ascend(&cacheItem{key: prefix}, func(item *cacheItem) bool {
		// Stop at the end of the prefix and skip pending tombstones.
		if !bytes.HasPrefix(item.key, prefix) {
			return false
		}
		searchItem := &cacheItem{key: item.key}
		if _, removed := t.remove.Get(searchItem); removed {
			return true
		}

		// Retain the visible pending key and value for sorted delivery.
		vals = append(vals, scanVal{
			key:   item.key,
			value: item.val,
		})
		return true
	})

	// Deliver the combined transaction values in key order.
	slices.SortFunc(vals, func(a, b scanVal) int {
		return bytes.Compare(a.key, b.key)
	})
	for i, val := range vals {
		// Deliver this transaction value to the prefix callback.
		if err := cb(val.key, val.value); err != nil {
			return err
		}

		// Release this delivered value from the sorted scan buffer.
		// release memory
		vals[i] = scanVal{}
	}
	return nil
}

// scanPrefixUnsorted implements ScanPrefix unsorted.
func (t *TXCache) scanPrefixUnsorted(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	// Capture the pending transaction trees and track emitted cache keys.
	t.mtx.RLock()
	snapRemove := t.remove
	snapSet := t.set
	t.mtx.RUnlock()
	seen := btree.NewBTreeG[*cacheItem](func(a, b *cacheItem) bool { return a.Less(b) })

	// Deliver underlying prefix keys with pending deletes and writes applied.
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

	// Deliver pending prefix keys absent from the underlying scan.
	snapSet.Ascend(&cacheItem{key: prefix}, func(item *cacheItem) bool {
		// Skip keys outside the prefix, deleted keys, and already emitted keys.
		if !bytes.HasPrefix(item.key, prefix) {
			return false
		}
		searchItem := &cacheItem{key: item.key}
		if _, ok := snapRemove.Get(searchItem); ok {
			return true
		}
		if _, ok := seen.Get(searchItem); ok {
			return true
		}

		// Deliver this pending cache value and stop if the callback fails.
		if err = cb(item.key, item.val); err != nil {
			return false
		}
		return true
	})
	return err
}
