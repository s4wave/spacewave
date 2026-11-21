package block_store_fillcache

import "sync/atomic"

// Counts are a Cache's counters since Open. Reads and fills together show
// whether the cache keeps what is read again: a block missed, queued and
// written should hit on its next read until eviction deletes it.
type Counts struct {
	// Hits are the reads the cache served.
	Hits uint64
	// Misses are the reads the cache could not serve, which go to the durable
	// store.
	Misses uint64
	// Queued are the fills queued for their write.
	Queued uint64
	// Dropped are the fills dropped because the queue was full.
	Dropped uint64
	// Written are the blocks written into the file.
	Written uint64
	// WriteFailed are the blocks whose write failed and were dropped.
	WriteFailed uint64
	// Evicted are the rows eviction deleted.
	Evicted uint64
}

// counters are the atomic counts behind Counts.
type counters struct {
	hits, misses, queued, dropped, written, writeFailed, evicted atomic.Uint64
}

// snapshot returns the current counts.
func (c *counters) snapshot() Counts {
	return Counts{
		Hits:        c.hits.Load(),
		Misses:      c.misses.Load(),
		Queued:      c.queued.Load(),
		Dropped:     c.dropped.Load(),
		Written:     c.written.Load(),
		WriteFailed: c.writeFailed.Load(),
		Evicted:     c.evicted.Load(),
	}
}

// Counts returns the cache's counters since Open.
func (c *Cache) Counts() Counts {
	return c.counts.snapshot()
}
