package block

// DecodedBlockCacheSnapshot is a point-in-time decoded-cache metrics snapshot.
type DecodedBlockCacheSnapshot struct {
	// MaxCost is the configured shared-cache budget.
	MaxCost int64
	// RemainingCost is the current unused budget reported by Ristretto.
	RemainingCost int64
	// RetainedCost is the current retained cost estimate.
	RetainedCost int64
	// Hits is the number of shared-cache hits.
	Hits uint64
	// Misses is the number of shared-cache misses.
	Misses uint64
	// Stores is the number of keys admitted by Ristretto.
	Stores uint64
	// Rejections is the number of set calls dropped or rejected by Ristretto.
	Rejections uint64
	// Evictions is the number of keys evicted by Ristretto.
	Evictions uint64
	// CostAdded is the sum of costs admitted by Ristretto.
	CostAdded uint64
	// CostEvicted is the sum of costs evicted by Ristretto.
	CostEvicted uint64
}
