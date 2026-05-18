package block

const (
	// DefaultDecodedBlockCacheMaxCost is the default decoded-object cache budget.
	DefaultDecodedBlockCacheMaxCost     int64 = 1_000_000_000
	defaultDecodedBlockCacheCounters    int64 = 100_000
	defaultDecodedBlockCacheBufferItems int64 = 64
)

// DecodedBlockCacheOptions configures a decoded-block cache owner.
type DecodedBlockCacheOptions struct {
	// MaxCost is the Ristretto cache budget.
	MaxCost int64
	// NumCounters controls Ristretto admission counter capacity.
	NumCounters int64
	// BufferItems controls Ristretto get-buffer size.
	BufferItems int64
	// Disabled bypasses decoded-object retention while preserving reads.
	Disabled bool
}

// DefaultDecodedBlockCacheOptions returns the production decoded-cache options.
func DefaultDecodedBlockCacheOptions() DecodedBlockCacheOptions {
	return DecodedBlockCacheOptions{
		MaxCost:     DefaultDecodedBlockCacheMaxCost,
		NumCounters: defaultDecodedBlockCacheCounters,
		BufferItems: defaultDecodedBlockCacheBufferItems,
	}
}

func (o DecodedBlockCacheOptions) normalize() DecodedBlockCacheOptions {
	if o.MaxCost == 0 {
		o.MaxCost = DefaultDecodedBlockCacheMaxCost
	}
	if o.NumCounters == 0 {
		o.NumCounters = defaultDecodedBlockCacheCounters
	}
	if o.BufferItems == 0 {
		o.BufferItems = defaultDecodedBlockCacheBufferItems
	}
	return o
}
