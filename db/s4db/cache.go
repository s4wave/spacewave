package s4db

import (
	"sync"
	"sync/atomic"
)

// cacheShards is the number of independently locked cache shards.
const cacheShards = 64

// cache holds decoded pages within a memory budget, in shards that each
// evict by CLOCK. Pages are never changed once written, and a page number is
// only reused after every snapshot that could read the old page has closed.
// Freeing drops the entry in the freeing handle; other handles clear the
// cache when they adopt the checkpoint that may reuse it.
type cache struct {
	// root holds the last root page read, so lookups start without taking
	// a shard lock.
	root atomic.Pointer[cacheSlot]
	// shards hold the pages by page number.
	shards [cacheShards]cacheShard
}

// cacheShard is one locked part of the cache.
type cacheShard struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// index maps page numbers to positions in ring.
	index map[uint64]int
	// ring holds the cached pages in clock order.
	ring []cacheSlot
	// free lists the empty positions in ring.
	free []int
	// hand is the next position the clock examines.
	hand int
	// bytes is the cost of the cached pages.
	bytes int
	// max bounds bytes.
	max int
}

// cacheSlot is one cached page.
type cacheSlot struct {
	// page is the page number.
	page uint64
	// n is the decoded page; nil marks an empty slot.
	n *node
	// used is set by a hit and cleared as the clock passes.
	used bool
}

// newCache returns a cache bounded to about bytes of decoded pages.
func newCache(bytes int) *cache {
	c := &cache{}
	for i := range c.shards {
		c.shards[i] = cacheShard{index: make(map[uint64]int), max: max(pageSize, bytes/cacheShards)}
	}
	return c
}

// shard returns the shard holding page.
func (c *cache) shard(page uint64) *cacheShard {
	return &c.shards[(page*0x9e3779b97f4a7c15)>>58]
}

// get returns a cached page.
func (c *cache) get(page uint64) *node {
	// Find the page in its shard.
	s := c.shard(page)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	i, ok := s.index[page]
	if !ok {
		return nil
	}

	// Mark it used so the clock passes it once.
	s.ring[i].used = true
	return s.ring[i].n
}

// put caches a page. A full shard evicts the pages the clock finds unused
// since it last passed until the page fits, so pages read once leave before
// pages read again.
func (c *cache) put(page uint64, n *node) {
	// Replace a cached page in place.
	s := c.shard(page)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if i, ok := s.index[page]; ok {
		s.bytes += n.cost - s.ring[i].n.cost
		s.ring[i].n = n
		return
	}

	// Evict until the page fits. A page larger than the budget still
	// enters an empty shard.
	for s.bytes > 0 && s.bytes+n.cost > s.max {
		s.evict()
	}

	// Take an empty slot or grow the ring.
	i := len(s.ring)
	if l := len(s.free); l != 0 {
		i, s.free = s.free[l-1], s.free[:l-1]
	} else {
		s.ring = append(s.ring, cacheSlot{})
	}
	s.ring[i] = cacheSlot{page: page, n: n}
	s.index[page] = i
	s.bytes += n.cost
}

// drop removes freed pages.
func (c *cache) drop(r run) {
	if root := c.root.Load(); root != nil && root.page >= r.start && root.page < r.end() {
		c.root.CompareAndSwap(root, nil)
	}
	for p := r.start; p < r.end(); p++ {
		s := c.shard(p)
		s.mtx.Lock()
		if i, ok := s.index[p]; ok {
			s.remove(i)
		}
		s.mtx.Unlock()
	}
}

// clear removes every page. Another process's checkpoint may reuse page
// numbers this handle never freed, so a handle adopting such a checkpoint
// clears the cache before it reads the new tree.
func (c *cache) clear() {
	c.root.Store(nil)
	for i := range c.shards {
		s := &c.shards[i]
		s.mtx.Lock()
		clear(s.index)
		s.ring, s.free, s.hand, s.bytes = nil, nil, 0, 0
		s.mtx.Unlock()
	}
}

// evict advances the clock to the first page unused since it last passed
// and removes it. The caller holds mtx and the shard holds a page.
func (s *cacheShard) evict() {
	for {
		i := s.hand
		s.hand = (s.hand + 1) % len(s.ring)
		switch {
		case s.ring[i].n == nil:
		case s.ring[i].used:
			s.ring[i].used = false
		default:
			s.remove(i)
			return
		}
	}
}

// remove empties slot i. The caller holds mtx.
func (s *cacheShard) remove(i int) {
	delete(s.index, s.ring[i].page)
	s.bytes -= s.ring[i].n.cost
	s.ring[i] = cacheSlot{}
	s.free = append(s.free, i)
}
