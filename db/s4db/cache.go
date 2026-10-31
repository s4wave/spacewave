package s4db

import (
	"sync"
)

// cacheKey identifies a cached block: the run and the block's offset.
type cacheKey struct {
	// run is the run id.
	run uint64
	// off is the block's file offset.
	off uint64
}

// cacheItem is one cached block.
type cacheItem struct {
	// key identifies the block.
	key cacheKey
	// data holds the block's bytes.
	data []byte
	// hits counts reads since insertion, capped at three; S3-FIFO promotes
	// a small-queue item with any hits and keeps a main-queue item with some.
	hits uint8
}

// blockCache holds run blocks, index partitions and filter partitions under
// a byte budget with S3-FIFO replacement: new blocks enter a small queue and
// move to the main queue only when read again before they leave it, so one
// pass over the file by a merge, the cleaner or an iterator never displaces
// the blocks lookups reuse. Evicted small-queue keys are remembered in a
// ghost set so a block read again soon after enters the main queue directly.
type blockCache struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// budget is the byte limit, and bytes the bytes held.
	budget, bytes int64
	// items maps each cached key to its item.
	items map[cacheKey]*cacheItem
	// small and main are the two FIFO queues, oldest first.
	small, main []*cacheItem
	// smallBytes is the bytes in small; it is kept at a tenth of the budget.
	smallBytes int64
	// ghost remembers keys evicted from small, bounded to ghostMax.
	ghost map[cacheKey]struct{}
	// ghostOrder holds ghost keys oldest first.
	ghostOrder []cacheKey
}

// ghostMax bounds the ghost set.
const ghostMax = 4096

// newBlockCache returns a cache of budget bytes.
func newBlockCache(budget int64) *blockCache {
	return &blockCache{budget: budget, items: make(map[cacheKey]*cacheItem), ghost: make(map[cacheKey]struct{})}
}

// get returns the cached block, or nil.
func (c *blockCache) get(k cacheKey) []byte {
	// Count the read on the item.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	it := c.items[k]
	if it == nil {
		return nil
	}
	if it.hits < 3 {
		it.hits++
	}
	return it.data
}

// put caches data under k, evicting as the budget requires.
func (c *blockCache) put(k cacheKey, data []byte) {
	// Skip a block the cache already holds or cannot hold.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if _, ok := c.items[k]; ok || int64(len(data)) > c.budget/4 {
		return
	}

	// Insert into main when the ghost remembers it, else into small.
	it := &cacheItem{key: k, data: data}
	c.items[k] = it
	c.bytes += int64(len(data))
	if _, ok := c.ghost[k]; ok {
		delete(c.ghost, k)
		c.main = append(c.main, it)
	} else {
		c.small = append(c.small, it)
		c.smallBytes += int64(len(data))
	}

	// Evict until within budget.
	for c.bytes > c.budget {
		if c.smallBytes > c.budget/10 && len(c.small) != 0 {
			c.evictSmall()
			continue
		}
		if len(c.main) == 0 {
			c.evictSmall()
			continue
		}
		c.evictMain()
	}
}

// evictSmall takes the oldest small item: promoted to main when it was read,
// else dropped and remembered in the ghost set. The queue's vacated slot is
// cleared, so the backing array does not keep a dropped block's bytes.
func (c *blockCache) evictSmall() {
	// Take the oldest.
	it := c.small[0]
	c.small[0] = nil
	c.small = c.small[1:]
	c.smallBytes -= int64(len(it.data))

	// Promote or drop.
	if it.hits > 0 {
		it.hits = 0
		c.main = append(c.main, it)
		return
	}
	delete(c.items, it.key)
	c.bytes -= int64(len(it.data))
	c.remember(it.key)
}

// evictMain takes the oldest main item: reinserted with one fewer hit when
// it was read, else dropped.
func (c *blockCache) evictMain() {
	// Take the oldest, reinserting it when it was read.
	it := c.main[0]
	c.main[0] = nil
	c.main = c.main[1:]
	if it.hits > 0 {
		it.hits--
		c.main = append(c.main, it)
		return
	}
	delete(c.items, it.key)
	c.bytes -= int64(len(it.data))
}

// remember adds k to the ghost set, forgetting the oldest past ghostMax.
func (c *blockCache) remember(k cacheKey) {
	c.ghost[k] = struct{}{}
	c.ghostOrder = append(c.ghostOrder, k)
	for len(c.ghostOrder) > ghostMax {
		delete(c.ghost, c.ghostOrder[0])
		c.ghostOrder = c.ghostOrder[1:]
	}
}

// drop forgets every block of run.
func (c *blockCache) drop(run uint64) {
	// Remove the items and compact the queues.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for k, it := range c.items {
		if k.run == run {
			delete(c.items, k)
			c.bytes -= int64(len(it.data))
			if c.inSmall(it) {
				c.smallBytes -= int64(len(it.data))
			}
			it.data = nil
		}
	}
	c.small = compactItems(c.small)
	c.main = compactItems(c.main)
}

// inSmall reports whether it waits in the small queue.
func (c *blockCache) inSmall(it *cacheItem) bool {
	for _, s := range c.small {
		if s == it {
			return true
		}
	}
	return false
}

// compactItems removes the dropped items from a queue.
func compactItems(q []*cacheItem) []*cacheItem {
	kept := q[:0]
	for _, it := range q {
		if it.data != nil {
			kept = append(kept, it)
		}
	}
	clear(q[len(kept):])
	return kept
}
