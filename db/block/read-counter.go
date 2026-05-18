package block

import (
	"context"
	"sync/atomic"
)

type readCounterContextKey struct{}

// ReadCounter records aggregate block reads for one logical operation.
type ReadCounter struct {
	// count is the number of block reads that returned without store error.
	count atomic.Uint64
	// bytes is the number of bytes returned by found block reads.
	bytes atomic.Uint64
	// misses is the number of block reads that returned not found without store error.
	misses atomic.Uint64
	// resourceGetBlockCount is the number of Resource SDK GetBlock calls.
	resourceGetBlockCount atomic.Uint64
	// resourceGetBlockRefs is the number of non-empty Resource SDK GetBlock refs.
	resourceGetBlockRefs atomic.Uint64
	// resourceGetBlockBytes is the number of bytes returned by found Resource SDK GetBlock calls.
	resourceGetBlockBytes atomic.Uint64
	// resourceGetBlockMisses is the number of Resource SDK GetBlock misses.
	resourceGetBlockMisses atomic.Uint64
	// decodedBlockUnmarshalCount is the number of decoded block unmarshals.
	decodedBlockUnmarshalCount atomic.Uint64
	// decodedBlockUnmarshalBytes is the number of bytes passed to decoded block unmarshalling.
	decodedBlockUnmarshalBytes atomic.Uint64
	// decodedBlockCacheAttempts is the number of decoded-block cache lookup attempts.
	decodedBlockCacheAttempts atomic.Uint64
	// decodedBlockCacheHits is the number of decoded-block cache hits.
	decodedBlockCacheHits atomic.Uint64
	// decodedBlockCacheMisses is the number of decoded-block cache misses.
	decodedBlockCacheMisses atomic.Uint64
	// decodedBlockClones is the number of clone-on-hit decoded block returns.
	decodedBlockClones atomic.Uint64
	// decodedBlockUncloneable is the number of cache candidates that could not be cloned.
	decodedBlockUncloneable atomic.Uint64
	// decodedBlockUncacheable is the number of decoded-block cache bypasses for missing exact keys.
	decodedBlockUncacheable atomic.Uint64
	// decodedBlockStoreAttempts is the number of decoded-block cache store submissions.
	decodedBlockStoreAttempts atomic.Uint64
	// decodedBlockStoreAccepted is the number of store submissions accepted by the cache buffer.
	decodedBlockStoreAccepted atomic.Uint64
	// decodedBlockStoreRejected is the number of store submissions rejected or dropped immediately.
	decodedBlockStoreRejected atomic.Uint64
	// decodedBlockStoreCost is the cost submitted for decoded-block cache retention.
	decodedBlockStoreCost atomic.Uint64
}

// WithReadCounter returns a context that records aggregate block reads.
func WithReadCounter(ctx context.Context) (context.Context, *ReadCounter) {
	if ctx == nil {
		ctx = context.Background()
	}
	counter := &ReadCounter{}
	return context.WithValue(ctx, readCounterContextKey{}, counter), counter
}

// Snapshot returns the current counter values.
func (c *ReadCounter) Snapshot() ReadCounterSnapshot {
	if c == nil {
		return ReadCounterSnapshot{}
	}
	return ReadCounterSnapshot{
		BlockReadCount:                 c.count.Load(),
		BlockReadBytes:                 c.bytes.Load(),
		BlockReadMissCount:             c.misses.Load(),
		ResourceGetBlockCount:          c.resourceGetBlockCount.Load(),
		ResourceGetBlockRefCount:       c.resourceGetBlockRefs.Load(),
		ResourceGetBlockBytes:          c.resourceGetBlockBytes.Load(),
		ResourceGetBlockMissCount:      c.resourceGetBlockMisses.Load(),
		DecodedBlockUnmarshalCount:     c.decodedBlockUnmarshalCount.Load(),
		DecodedBlockUnmarshalBytes:     c.decodedBlockUnmarshalBytes.Load(),
		DecodedBlockCacheAttemptCount:  c.decodedBlockCacheAttempts.Load(),
		DecodedBlockCacheHitCount:      c.decodedBlockCacheHits.Load(),
		DecodedBlockCacheMissCount:     c.decodedBlockCacheMisses.Load(),
		DecodedBlockCloneCount:         c.decodedBlockClones.Load(),
		DecodedBlockUncloneableCount:   c.decodedBlockUncloneable.Load(),
		DecodedBlockUncacheableCount:   c.decodedBlockUncacheable.Load(),
		DecodedBlockStoreAttemptCount:  c.decodedBlockStoreAttempts.Load(),
		DecodedBlockStoreAcceptedCount: c.decodedBlockStoreAccepted.Load(),
		DecodedBlockStoreRejectedCount: c.decodedBlockStoreRejected.Load(),
		DecodedBlockStoreCost:          c.decodedBlockStoreCost.Load(),
	}
}

func recordReadCounter(ctx context.Context, found bool, bytes int) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.count.Add(1)
	if found {
		counter.bytes.Add(uint64(bytes))
		return
	}
	counter.misses.Add(1)
}

// RecordResourceGetBlock records a Resource SDK GetBlock call.
func RecordResourceGetBlock(ctx context.Context, ref *BlockRef, found bool, bytes int) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.resourceGetBlockCount.Add(1)
	if ref != nil && !ref.GetEmpty() {
		counter.resourceGetBlockRefs.Add(1)
	}
	if found {
		counter.resourceGetBlockBytes.Add(uint64(bytes))
		return
	}
	counter.resourceGetBlockMisses.Add(1)
}

func recordDecodedBlockUnmarshal(ctx context.Context, bytes int) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockUnmarshalCount.Add(1)
	counter.decodedBlockUnmarshalBytes.Add(uint64(bytes))
}

func recordDecodedBlockCacheMiss(ctx context.Context) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockCacheAttempts.Add(1)
	counter.decodedBlockCacheMisses.Add(1)
}

// RecordDecodedBlockCacheHit records a clone-safe decoded-block cache hit.
func RecordDecodedBlockCacheHit(ctx context.Context, cloned bool) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockCacheAttempts.Add(1)
	counter.decodedBlockCacheHits.Add(1)
	if cloned {
		counter.decodedBlockClones.Add(1)
	}
}

// RecordDecodedBlockUncloneable records a decoded-block cache candidate that cannot be cloned.
func RecordDecodedBlockUncloneable(ctx context.Context) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockUncloneable.Add(1)
}

// RecordDecodedBlockUncacheable records a decoded-block cache bypass without an exact key.
func RecordDecodedBlockUncacheable(ctx context.Context) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockUncacheable.Add(1)
}

func recordDecodedBlockCacheStore(ctx context.Context, accepted bool, cost int64) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockStoreAttempts.Add(1)
	if accepted {
		counter.decodedBlockStoreAccepted.Add(1)
	}
	if cost > 0 {
		counter.decodedBlockStoreCost.Add(uint64(cost))
	}
}

func recordDecodedBlockCacheRejected(ctx context.Context) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.decodedBlockStoreRejected.Add(1)
}
