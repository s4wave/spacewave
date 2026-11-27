package s4db

import (
	"sync"
	"sync/atomic"
)

// cacheKey identifies a cached block: the run and the block's offset.
type cacheKey struct {
	// run is the run id.
	run uint64
	// off is the block's file offset.
	off uint64
}

// cacheBlock is what a reader sees of a cached block.
type cacheBlock struct {
	// data holds the block's bytes. They never change.
	data []byte
	// hits counts reads since insertion, capped at three. S3-FIFO promotes
	// a small-queue item with any hits and keeps a main-queue item with
	// some. Readers count without the lock, so it is atomic.
	hits atomic.Uint32
}

// cacheItem is a cached block's place in the queues.
type cacheItem struct {
	// key identifies the block.
	key cacheKey
	// block holds the bytes and the hit count. A dropped run's item has
	// none, so the bytes are free while the item still waits in its queue.
	block *cacheBlock
	// small reports that the item waits in the small queue.
	small bool
}

// blockCache holds run blocks, index partitions and filter partitions under
// a byte budget with S3-FIFO replacement: new blocks enter a small queue and
// move to the main queue only when read again before they leave it, so one
// pass over the file by a merge, the cleaner or an iterator never displaces
// the blocks lookups reuse. Evicted small-queue keys are remembered in a
// ghost set so a block read again soon after enters the main queue directly.
//
// A read takes no lock. It finds the block in a map built for readers that
// run beside each other, and writes only the block's hit count, and that only
// until the count reaches its cap. So the parallel lookups of one batch read
// cached blocks without waiting on each other or on an insert. Inserts,
// evictions and drops take the lock.
type blockCache struct {
	// blocks maps each cached cacheKey to its *cacheBlock, for get. It
	// holds exactly the blocks of items, and changes with it under mtx.
	blocks sync.Map
	// mtx guards the fields below.
	mtx sync.Mutex
	// budget is the byte limit, and bytes the bytes held.
	budget, bytes int64
	// items maps each cached run, then each block offset, to its item, so
	// dropping a run finds its items alone.
	items map[uint64]map[uint64]*cacheItem
	// small and main are the two FIFO queues, oldest first. A dropped run's
	// items stay queued without their block, and eviction passes over them.
	small, main []*cacheItem
	// smallBytes is the bytes in small. It is kept at a tenth of the
	// budget.
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
	return &blockCache{budget: budget, items: make(map[uint64]map[uint64]*cacheItem), ghost: make(map[cacheKey]struct{})}
}

// get returns the cached block, or nil.
func (c *blockCache) get(k cacheKey) []byte {
	// Find the block beside other readers.
	v, ok := c.blocks.Load(k)
	if !ok {
		return nil
	}
	b := v.(*cacheBlock)

	// Count the read up to the cap. A reader that loses the swap reads the
	// count again, so the cap holds under any number of readers.
	for {
		h := b.hits.Load()
		if h >= 3 || b.hits.CompareAndSwap(h, h+1) {
			break
		}
	}
	return b.data
}

// put caches data under k, evicting as the budget requires.
func (c *blockCache) put(k cacheKey, data []byte) {
	// Skip a block the cache already holds or cannot hold.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if _, ok := c.items[k.run][k.off]; ok || int64(len(data)) > c.budget/4 {
		return
	}

	// Insert into main when the ghost remembers it, else into small.
	it := &cacheItem{key: k, block: &cacheBlock{data: data}}
	run := c.items[k.run]
	if run == nil {
		run = make(map[uint64]*cacheItem)
		c.items[k.run] = run
	}
	run[k.off] = it
	c.blocks.Store(k, it.block)
	c.bytes += int64(len(data))
	if _, ok := c.ghost[k]; ok {
		delete(c.ghost, k)
		c.main = append(c.main, it)
	} else {
		it.small = true
		c.small = append(c.small, it)
		c.smallBytes += int64(len(data))
	}

	// Evict until the cache is within budget.
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
	// Take the oldest small item, passing over a dropped run's item.
	it := c.small[0]
	c.small[0] = nil
	c.small = c.small[1:]
	if it.block == nil {
		return
	}
	c.smallBytes -= int64(len(it.block.data))

	// Promote or drop.
	if it.block.hits.Load() > 0 {
		it.block.hits.Store(0)
		it.small = false
		c.main = append(c.main, it)
		return
	}
	c.forget(it)
	c.remember(it.key)
}

// evictMain takes the oldest main item: reinserted with one fewer hit when
// it was read, else dropped.
func (c *blockCache) evictMain() {
	// Take the oldest main item, passing over a dropped run's item and
	// reinserting one that was read.
	it := c.main[0]
	c.main[0] = nil
	c.main = c.main[1:]
	switch {
	case it.block == nil:
		return
	case it.block.hits.Load() > 0:
		// Readers only raise the count, so it is still above zero here.
		it.block.hits.Add(^uint32(0))
		c.main = append(c.main, it)
		return
	}
	c.forget(it)
}

// forget removes it from the items, the readers' map and the byte count.
func (c *blockCache) forget(it *cacheItem) {
	// Leave the run's items, and drop a run left empty.
	run := c.items[it.key.run]
	delete(run, it.key.off)
	if len(run) == 0 {
		delete(c.items, it.key.run)
	}

	// Leave the readers' map and the count.
	c.blocks.Delete(it.key)
	c.bytes -= int64(len(it.block.data))
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

// drop forgets every block of run, touching only its items: each leaves its
// queue when eviction reaches it.
func (c *blockCache) drop(run uint64) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for _, it := range c.items[run] {
		c.bytes -= int64(len(it.block.data))
		if it.small {
			c.smallBytes -= int64(len(it.block.data))
		}
		c.blocks.Delete(it.key)
		it.block = nil
	}
	delete(c.items, run)
}
