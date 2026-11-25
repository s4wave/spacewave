package block_store_fillcache

import "sync/atomic"

// Counts are a Cache's counters since Open. Reads and fills together show
// whether the cache keeps what is read again: a block missed, queued and
// written should hit on its next read until eviction drops its pack.
type Counts struct {
	// Hits are the reads the cache served.
	Hits uint64
	// Misses are the reads the cache could not serve, which go to the durable
	// store.
	Misses uint64
	// Queued are the fills queued for their write.
	Queued uint64
	// Dropped are the fills dropped because the queue was full or the packs
	// were not loaded.
	Dropped uint64
	// Written are the blocks appended to the packs.
	Written uint64
	// WriteFailed are the blocks whose write failed and were dropped.
	WriteFailed uint64
	// Evicted are the packs eviction dropped.
	Evicted uint64
	// Carried are the blocks eviction carried forward into the open pack.
	Carried uint64
}

// counters are the atomic counts behind Counts that the packs do not keep.
type counters struct {
	hits, misses, queued, dropped, written, writeFailed atomic.Uint64
}

// Counts returns the cache's counters since Open.
func (c *Cache) Counts() Counts {
	packs := c.packs.Counts()
	return Counts{
		Hits:        c.counts.hits.Load(),
		Misses:      c.counts.misses.Load(),
		Queued:      c.counts.queued.Load(),
		Dropped:     c.counts.dropped.Load(),
		Written:     c.counts.written.Load(),
		WriteFailed: c.counts.writeFailed.Load(),
		Evicted:     packs.Evicted,
		Carried:     packs.Carried,
	}
}
