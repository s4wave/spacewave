package block

const (
	// DefaultDecodedBlockCacheMaxCost is the default decoded-object cache
	// budget. The process-wide pool uses it for every block store together.
	DefaultDecodedBlockCacheMaxCost int64 = 256 << 20
)

// DecodedBlockCacheOptions configures a decoded-block cache pool.
type DecodedBlockCacheOptions struct {
	// MaxCost is the pool's budget in bytes of decoded blocks.
	MaxCost int64
	// Disabled bypasses decoded-object retention while preserving reads.
	Disabled bool
}

// DefaultDecodedBlockCacheOptions returns the production decoded-cache options.
func DefaultDecodedBlockCacheOptions() DecodedBlockCacheOptions {
	return DecodedBlockCacheOptions{MaxCost: DefaultDecodedBlockCacheMaxCost}
}

// normalize fills zero fields with the defaults.
func (o DecodedBlockCacheOptions) normalize() DecodedBlockCacheOptions {
	if o.MaxCost == 0 {
		o.MaxCost = DefaultDecodedBlockCacheMaxCost
	}
	return o
}
