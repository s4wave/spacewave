package blob

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

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

// setLimit changes the retained-byte budget. Existing entries remain until a
// cache miss needs room, and the latest chunk is always retained.
func (c *chunkReadCache) setLimit(limit int) {
	c.limit = limit
}

// read reads a chunk range, enabling bounded read-ahead for streaming requests.
func (c *chunkReadCache) read(ctx context.Context, chunks *sbset.SubBlockSet, buf []byte, start, hint int) (int, int, error) {
	// Enable read-ahead before locating the requested chunk.
	if c.ahead == nil && len(buf) >= chunkReadAheadMinRead && chunks.Len() > 1 {
		c.ahead = newChunkReadAhead(ctx, chunks)
	}

	// Resolve the requested range through the retained chunks and window.
	return readFromChunks(ctx, chunks, buf, start, hint, c)
}

// fetch returns cached data or fetches and retains the requested chunk.
// The caller has checked that the chunk size fits in an int.
func (c *chunkReadCache) fetch(ctx context.Context, chunk *Chunk, cursor *block.Cursor, idx int) ([]byte, error) {
	// Reuse retained bytes, or make room before fetching a missing chunk.
	if data, ok := c.get(idx); ok {
		return data, nil
	}
	c.evict(int(chunk.GetSize())) //nolint:gosec // the caller validated the chunk bounds.

	// Fetch through the streaming window or the uncached cursor path.
	var data []byte
	var err error
	if c.ahead != nil {
		data, err = c.ahead.read(idx)
	} else {
		data, err = fetchChunkDataNoCursorCache(ctx, chunk, cursor, idx, nil)
	}
	if err != nil {
		return nil, err
	}

	// Retain the fetched bytes as the most recently used chunk.
	c.add(idx, data)
	return data, nil
}

// stopReadAhead cancels and joins the streaming window, keeping retained chunks.
func (c *chunkReadCache) stopReadAhead() {
	if c.ahead != nil {
		c.ahead.close()
		c.ahead = nil
	}
}

// close joins outstanding reads before releasing retained buffers and the budget.
func (c *chunkReadCache) close() {
	c.stopReadAhead()
	*c = chunkReadCache{}
}

// get returns the retained data for idx and marks it most recently used.
func (c *chunkReadCache) get(idx int) ([]byte, bool) {
	// Locate the chunk's entry.
	i := slices.IndexFunc(c.entries, func(entry chunkReadCacheEntry) bool {
		return entry.idx == idx
	})
	if i < 0 {
		return nil, false
	}

	// Move the entry to the most recently used position and return its data.
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
