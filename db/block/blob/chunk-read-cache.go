package blob

import "slices"

// chunkReadCache retains recently read chunk data for one Reader, least
// recently used first. Retained data stays within limit bytes, except that the
// latest chunk is always kept so repeated small reads inside one chunk do not
// fetch it again. Only the owning Reader accesses the cache.
type chunkReadCache struct {
	// limit is the byte budget for retained chunk data.
	limit int
	// size is the byte total of entries.
	size int
	// entries holds retained chunks, least recently used first.
	entries []chunkReadCacheEntry
	// ahead is the streaming read-ahead window, or nil for random access.
	ahead *chunkReadAhead
}

// chunkReadCacheEntry is the data of one chunk by index.
type chunkReadCacheEntry struct {
	idx  int
	data []byte
}

// get returns the retained data for idx and marks it most recently used.
func (c *chunkReadCache) get(idx int) ([]byte, bool) {
	i := slices.IndexFunc(c.entries, func(entry chunkReadCacheEntry) bool {
		return entry.idx == idx
	})
	if i < 0 {
		return nil, false
	}
	entry := c.entries[i]
	copy(c.entries[i:], c.entries[i+1:])
	c.entries[len(c.entries)-1] = entry
	return entry.data, true
}

// evict drops least recently used chunks until incoming more bytes fit within
// limit. Call it before fetching so the evicted data is released first.
func (c *chunkReadCache) evict(incoming int) {
	drop := 0
	for drop < len(c.entries) && c.size+incoming > c.limit {
		c.size -= len(c.entries[drop].data)
		drop++
	}
	c.entries = slices.Delete(c.entries, 0, drop)
}

// add retains data for idx as the most recently used chunk.
func (c *chunkReadCache) add(idx int, data []byte) {
	c.entries = append(c.entries, chunkReadCacheEntry{idx: idx, data: data})
	c.size += len(data)
}
