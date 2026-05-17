package block

// ReadCounterSnapshot is a point-in-time copy of block read counters.
type ReadCounterSnapshot struct {
	// BlockReadCount is the number of block reads that returned without store error.
	BlockReadCount uint64
	// BlockReadBytes is the number of bytes returned by found block reads.
	BlockReadBytes uint64
	// BlockReadMissCount is the number of block reads that returned not found without store error.
	BlockReadMissCount uint64
	// ResourceGetBlockCount is the number of Resource SDK GetBlock calls.
	ResourceGetBlockCount uint64
	// ResourceGetBlockRefCount is the number of non-empty refs passed to Resource SDK GetBlock.
	ResourceGetBlockRefCount uint64
	// ResourceGetBlockBytes is the number of bytes returned by found Resource SDK GetBlock calls.
	ResourceGetBlockBytes uint64
	// ResourceGetBlockMissCount is the number of Resource SDK GetBlock calls that missed.
	ResourceGetBlockMissCount uint64
	// DecodedBlockUnmarshalCount is the number of decoded block unmarshals.
	DecodedBlockUnmarshalCount uint64
	// DecodedBlockUnmarshalBytes is the number of bytes passed to decoded block unmarshalling.
	DecodedBlockUnmarshalBytes uint64
	// DecodedBlockCacheAttemptCount is the number of decoded-block cache lookup attempts.
	DecodedBlockCacheAttemptCount uint64
	// DecodedBlockCacheHitCount is the number of decoded-block cache hits.
	DecodedBlockCacheHitCount uint64
	// DecodedBlockCacheMissCount is the number of decoded-block cache misses.
	DecodedBlockCacheMissCount uint64
	// DecodedBlockCloneCount is the number of clone-on-hit decoded block returns.
	DecodedBlockCloneCount uint64
	// DecodedBlockUncloneableCount is the number of cache candidates that could not be cloned.
	DecodedBlockUncloneableCount uint64
	// DecodedBlockUncacheableCount is the number of decoded-block cache bypasses for missing exact keys.
	DecodedBlockUncacheableCount uint64
}
