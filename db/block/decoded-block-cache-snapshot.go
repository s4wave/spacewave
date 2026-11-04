package block

// DecodedBlockCacheSnapshot is a point-in-time decoded-cache metrics snapshot.
type DecodedBlockCacheSnapshot struct {
	// MaxCost is the configured shared-cache budget.
	MaxCost int64
	// RemainingCost is the current unused budget of the pool.
	RemainingCost int64
	// RetainedCost is the current retained cost estimate.
	RetainedCost int64
	// Entries is the number of decoded blocks held.
	Entries int64
	// ReadCost is the part of RetainedCost charged for immutable reader
	// metadata: child links attached to entries by ReadCursor.
	ReadCost int64
	// ReadSlots is the number of entries with reader metadata attached.
	ReadSlots int64
	// Hits is the number of shared-cache hits.
	Hits uint64
	// Misses is the number of shared-cache misses.
	Misses uint64
	// Stores is the number of stores into the pool.
	Stores uint64
	// Rejections is the number of stores refused for exceeding a shard budget.
	Rejections uint64
	// Evictions is the number of entries evicted.
	Evictions uint64
	// CostAdded is the sum of costs stored.
	CostAdded int64
	// CostEvicted is the sum of costs evicted.
	CostEvicted int64
}
