//go:build !tinygo

package block_store_s3

// HTTPRangeCacheCounts counts cache hits, origin misses and direct fallbacks.
type HTTPRangeCacheCounts struct {
	// Requests counts hit, miss and fallback requests, in that order.
	Requests [3]uint64
	// Bytes counts hit, miss and fallback bytes, in that order.
	Bytes [3]uint64
}
