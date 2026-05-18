package block

import "sync"

type decodedBlockFrontCache struct {
	mtx     sync.Mutex
	entries map[decodedBlockCacheKey]Block
}

func newDecodedBlockFrontCache() *decodedBlockFrontCache {
	return &decodedBlockFrontCache{
		entries: make(map[decodedBlockCacheKey]Block),
	}
}

func (c *decodedBlockFrontCache) lookup(key decodedBlockCacheKey) Block {
	if c == nil {
		return nil
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.entries[key]
}

func (c *decodedBlockFrontCache) store(key decodedBlockCacheKey, blk Block) {
	if c == nil || blk == nil {
		return
	}
	c.mtx.Lock()
	c.entries[key] = blk
	c.mtx.Unlock()
}
