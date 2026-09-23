package block

const (
	// DefaultDecodedBlockCacheMaxCost is the default decoded-object cache
	// budget. The process-wide pool uses it for every block store together.
	DefaultDecodedBlockCacheMaxCost int64 = 256 << 20
	// defaultDecodedBlockCacheCounters sizes admission counters at about ten
	// per entry for small IAVL nodes near 1 KiB.
	defaultDecodedBlockCacheCounters    int64 = 1 << 22
	defaultDecodedBlockCacheBufferItems int64 = 64
)

// DecodedBlockCacheOptions configures a decoded-block cache pool.
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

// normalize fills zero fields with the defaults.
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
