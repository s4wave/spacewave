//go:build !tinygo

package block_store_s3

import (
	"container/list"
	"sync"
)

// coldCache bounds retained values by byte or key cost, evicting the oldest
// access first. Its lock guards only in-memory list and map changes.
type coldCache[K comparable, V any] struct {
	// mtx guards all mutable cache state.
	mtx sync.Mutex
	// entries locates an item in the LRU ordering.
	entries map[K]*list.Element
	// order holds the most recently used value first.
	order list.List
	// used is the total resident charge.
	used int
	// limit is the immutable maximum charge.
	limit int
}

// coldCacheEntry retains a value and its accounting information.
type coldCacheEntry[K comparable, V any] struct {
	// key identifies the map entry to remove on eviction.
	key K
	// value is the immutable cached entry.
	value V
	// charge is the cost of retaining value.
	charge int
}

// newColdCache creates an empty cache with a positive limit.
func newColdCache[K comparable, V any](limit int) *coldCache[K, V] {
	return &coldCache[K, V]{entries: make(map[K]*list.Element), limit: limit}
}

// get marks a hit used and returns the immutable value, even across eviction.
func (c *coldCache[K, V]) get(key K) (V, bool) {
	return c.lookup(key, true)
}

// peek borrows a value without changing its place in the eviction order.
func (c *coldCache[K, V]) peek(key K) (V, bool) {
	return c.lookup(key, false)
}

// lookup copies an immutable value under the lock; only its owner may touch it.
func (c *coldCache[K, V]) lookup(key K, touch bool) (V, bool) {
	// A miss does not create state or run an eviction.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	e := c.entries[key]
	if e == nil {
		var zero V
		return zero, false
	}

	// A move borrowing a user tail must not reorder that cache.
	if touch {
		c.order.MoveToFront(e)
	}
	return e.Value.(coldCacheEntry[K, V]).value, true
}

// put caches a value while never exceeding the charge limit. Warm fills only
// use spare capacity; they cannot evict anything a request already read.
func (c *coldCache[K, V]) put(key K, value V, charge int, spareOnly bool) {
	// Existing immutable entries are never installed twice.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.entries[key] != nil || charge > c.limit || spareOnly && charge > c.limit-c.used {
		return
	}

	// Evict from the LRU end until the entire new charge fits.
	for charge > c.limit-c.used {
		old := c.order.Back()
		entry := c.order.Remove(old).(coldCacheEntry[K, V])
		delete(c.entries, entry.key)
		c.used -= entry.charge
	}
	c.entries[key] = c.order.PushFront(coldCacheEntry[K, V]{key: key, value: value, charge: charge})
	c.used += charge
}

// room reports whether the entire charge fits without eviction.
func (c *coldCache[K, V]) room(charge uint64) bool {
	// Installation rechecks after racing foreground loads.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return charge <= uint64(c.limit-c.used) //nolint:gosec // used is at most limit.
}
