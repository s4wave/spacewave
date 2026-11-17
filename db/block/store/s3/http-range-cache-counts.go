//go:build !tinygo

package block_store_s3

// HTTPRangeCacheCounts counts reads by result and gives the breaker's state.
type HTTPRangeCacheCounts struct {
	// Requests counts hit, miss, fallback, skipped, deadline and rejected
	// requests, in that order. Skipped reads went direct while the breaker was
	// open; deadline reads went direct because the caller's deadline was too
	// near; rejected reads went direct because bytes the Worker served earlier
	// in the same block read failed the block's hash.
	Requests [6]uint64
	// Bytes counts the same results' bytes, in the same order.
	Bytes [6]uint64
	// Breaker is the breaker's state when the counts were read.
	Breaker HTTPRangeCacheBreaker
}
