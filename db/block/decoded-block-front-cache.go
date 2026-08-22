package block

import "sync"

// decodedBlockFrontCache caches decoded blocks for the front store so
// repeated reads skip decoding. All fields are guarded by mtx.
type decodedBlockFrontCache struct {
	mtx     sync.Mutex
	entries map[decodedBlockCacheKey]Block
}

// newDecodedBlockFrontCache constructs an empty front cache.
func newDecodedBlockFrontCache() *decodedBlockFrontCache {
	return &decodedBlockFrontCache{
		entries: make(map[decodedBlockCacheKey]Block),
	}
}

// lookup returns the cached block for the key, or nil.
func (c *decodedBlockFrontCache) lookup(key decodedBlockCacheKey) Block {
	if c == nil {
		return nil
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.entries[key]
}

// store caches the block for the key.
func (c *decodedBlockFrontCache) store(key decodedBlockCacheKey, blk Block) {
	if c == nil || blk == nil {
		return
	}
	c.mtx.Lock()
	c.entries[key] = blk
	c.mtx.Unlock()
}

// invalidateRef drops every cached entry belonging to the ref.
func (c *decodedBlockFrontCache) invalidateRef(refKey string) {
	if c == nil || refKey == "" {
		return
	}
	c.mtx.Lock()
	for key := range c.entries {
		if key.ref == refKey {
			delete(c.entries, key)
		}
	}
	c.mtx.Unlock()
}

// clear empties the cache.
func (c *decodedBlockFrontCache) clear() {
	if c == nil {
		return
	}
	c.mtx.Lock()
	c.entries = make(map[decodedBlockCacheKey]Block)
	c.mtx.Unlock()
}
