package block_store_memcache

// Stats counts what a Cache holds and has served.
type Stats struct {
	// Blocks counts the blocks held.
	Blocks int
	// Bytes is the memory they take, as the bound counts it.
	Bytes int64
	// Hits counts the reads the cache served.
	Hits uint64
	// Fills counts the blocks copied in.
	Fills uint64
	// Evicted counts the blocks evicted for the bound.
	Evicted uint64
}
