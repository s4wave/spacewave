package block

import (
	"context"
	"sync/atomic"
	"time"
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
	// nanos is the time spent in the store reads counted by count.
	nanos atomic.Int64
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

// ContextReadCounter returns the read counter WithReadCounter attached to
// ctx, or nil.
func ContextReadCounter(ctx context.Context) *ReadCounter {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	return counter
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
		BlockReadTime:                  time.Duration(c.nanos.Load()),
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

// recordRead records a block read that took elapsed and whether it was
// found in the store. A nil counter records nothing.
func (c *ReadCounter) recordRead(found bool, bytes int, elapsed time.Duration) {
	// Skip a context with no counter.
	if c == nil {
		return
	}

	// Count the store read, its time and its result.
	c.count.Add(1)
	c.nanos.Add(int64(elapsed))
	if found {
		c.bytes.Add(nonNegativeReadBytes(bytes))
		return
	}
	c.misses.Add(1)
}

// RecordResourceGetBlock records a Resource SDK GetBlock call of ref and
// whether it found bytes bytes. A nil counter records nothing.
func (c *ReadCounter) RecordResourceGetBlock(ref *BlockRef, found bool, bytes int) {
	// Skip a context with no counter.
	if c == nil {
		return
	}

	// Count the Resource SDK read and its result.
	c.resourceGetBlockCount.Add(1)
	if ref != nil && !ref.GetEmpty() {
		c.resourceGetBlockRefs.Add(1)
	}
	if found {
		c.resourceGetBlockBytes.Add(nonNegativeReadBytes(bytes))
		return
	}
	c.resourceGetBlockMisses.Add(1)
}

// recordUnmarshal records a decoded block unmarshal of bytes bytes. A nil
// counter records nothing.
func (c *ReadCounter) recordUnmarshal(bytes int) {
	if c == nil {
		return
	}
	c.decodedBlockUnmarshalCount.Add(1)
	c.decodedBlockUnmarshalBytes.Add(nonNegativeReadBytes(bytes))
}

// recordCacheMiss records a decoded-block cache lookup that missed. A nil
// counter records nothing.
func (c *ReadCounter) recordCacheMiss() {
	if c == nil {
		return
	}
	c.decodedBlockCacheAttempts.Add(1)
	c.decodedBlockCacheMisses.Add(1)
}

// recordCacheHit records a decoded-block cache hit, and whether the caller
// got a clone of the cached block. A nil counter records nothing.
func (c *ReadCounter) recordCacheHit(cloned bool) {
	// Skip a context with no counter.
	if c == nil {
		return
	}

	// Count the cache hit and its returned clone.
	c.decodedBlockCacheAttempts.Add(1)
	c.decodedBlockCacheHits.Add(1)
	if cloned {
		c.decodedBlockClones.Add(1)
	}
}

// recordUncloneable records a decoded-block cache candidate that cannot be
// cloned. A nil counter records nothing.
func (c *ReadCounter) recordUncloneable() {
	if c == nil {
		return
	}
	c.decodedBlockUncloneable.Add(1)
}

// recordUncacheable records a decoded-block cache bypass for a block without
// an exact key. A nil counter records nothing.
func (c *ReadCounter) recordUncacheable() {
	if c == nil {
		return
	}
	c.decodedBlockUncacheable.Add(1)
}

// recordCacheStore records a decoded-block cache store submission, whether
// the pool accepted or rejected it, and its cost. A nil counter records
// nothing.
func (c *ReadCounter) recordCacheStore(accepted bool, cost int64) {
	// Skip a context with no counter.
	if c == nil {
		return
	}

	// Count the submission, its outcome and its retained cost.
	c.decodedBlockStoreAttempts.Add(1)
	if accepted {
		c.decodedBlockStoreAccepted.Add(1)
	} else {
		c.decodedBlockStoreRejected.Add(1)
	}
	if cost > 0 {
		c.decodedBlockStoreCost.Add(uint64(cost))
	}
}

// nonNegativeReadBytes converts a byte count to the unsigned counter type,
// counting a negative value as zero.
func nonNegativeReadBytes(value int) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value) //nolint:gosec // the positive check protects the unsigned counter from negative callback values.
}
