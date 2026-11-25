package block_store_fillcache

// Options configures a Cache.
type Options struct {
	// LimitBytes bounds the bytes of the cache's packs; past it, eviction drops
	// the oldest pack after carrying its read blocks forward. Zero uses
	// DefaultLimitBytes.
	LimitBytes int64
	// QueueBytes bounds the block bytes awaiting their write. A fill past it is
	// dropped, and the next read of the block queues it again. Zero uses
	// DefaultQueueBytes.
	QueueBytes int64
}
