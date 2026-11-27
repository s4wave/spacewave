package block_store_writeback

import (
	"sync/atomic"
	"time"
)

// evictBatchStartBlocks is the initial block limit of one eviction batch.
// A batch reads at most the store's current
// block limit of entries, and stops before an entry that would take it past
// the batch byte bound, evictBatchBytes by default. The limit starts at
// evictBatchStartBlocks on each open and follows the time the batch's drop
// write takes, as nextEvictLimit describes, between the batch's least and
// most blocks, evictBatchMinBlocks and evictBatchMaxBlocks by default.
//
// The drop write holds the store's writer for a time that grows with the
// blocks it drops, not their bytes: each tombstone is a row delete that may
// rebalance its B-tree page and read its neighbors. On a store much larger
// than the page cache that costs from a quarter of a millisecond to several
// milliseconds a block, so a fixed count large enough to move many bytes
// would hold the writer for seconds on a slow disk.
const (
	evictBatchStartBlocks = 1024
	evictBatchMinBlocks   = 128
	evictBatchMaxBlocks   = 8192
	evictBatchBytes       = 32 << 20
)

// evictWriteTarget is the time a batch's drop write aims to take by default,
// half the longest a background write may hold the writer, so a batch whose
// blocks cost twice the last batch's still releases it within a second.
const evictWriteTarget = 500 * time.Millisecond

// EvictBatch bounds the eviction batches of every store in the process, as
// SetEvictBatch sets them. A zero field keeps its default.
type EvictBatch struct {
	// MinBlocks is the least block limit a batch shrinks to, 128 by default.
	MinBlocks int
	// MaxBlocks is the most blocks a batch takes, 8192 by default.
	MaxBlocks int
	// MaxBytes is the most bytes a batch takes past its first entry, 32 MiB
	// by default.
	MaxBytes int64
	// WriteTarget is the time a batch's drop write aims to take, 500 ms by
	// default. The next batch's block limit scales to it.
	WriteTarget time.Duration
}

// evictBatchConfig holds the bounds SetEvictBatch set, nil for the defaults.
var evictBatchConfig atomic.Pointer[EvictBatch]

// SetEvictBatch sets the bounds of the eviction batches of every store in the
// process. A pass running reads them at its next batch, which clamps the
// store's block limit into the new bounds, so lowering them applies within a
// batch. A zero field keeps its default, a negative one too, and a least
// block limit past the most is lowered to it.
func SetEvictBatch(b EvictBatch) {
	b = b.normalize()
	evictBatchConfig.Store(&b)
}

// loadEvictBatch returns the bounds in force.
func loadEvictBatch() EvictBatch {
	// Use the bounds SetEvictBatch stored, else the defaults.
	if b := evictBatchConfig.Load(); b != nil {
		return *b
	}
	return EvictBatch{}.normalize()
}

// normalize fills the defaults of the zero or negative fields and keeps
// MinBlocks at most MaxBlocks.
func (b EvictBatch) normalize() EvictBatch {
	// Fill each unset field with its default, then order the block bounds.
	if b.MinBlocks <= 0 {
		b.MinBlocks = evictBatchMinBlocks
	}
	if b.MaxBlocks <= 0 {
		b.MaxBlocks = evictBatchMaxBlocks
	}
	if b.MaxBytes <= 0 {
		b.MaxBytes = evictBatchBytes
	}
	if b.WriteTarget <= 0 {
		b.WriteTarget = evictWriteTarget
	}
	b.MinBlocks = min(b.MinBlocks, b.MaxBlocks)
	return b
}

// clamp returns limit within the batch's block bounds.
func (b EvictBatch) clamp(limit int) int {
	return min(max(limit, b.MinBlocks), b.MaxBlocks)
}

// nextEvictLimit returns the block limit of the batch after one that dropped
// dropped blocks under limit in elapsed: the count that would take b's write
// target at the same cost a block, moving at most a factor of two from limit
// and kept within b's block bounds. A batch that dropped nothing measured
// nothing and keeps limit. elapsed includes the write's wait for the writer
// turn, so a busy writer shrinks the batches too.
func nextEvictLimit(b EvictBatch, limit, dropped int, elapsed time.Duration) int {
	// Keep the limit when the batch measured nothing.
	if dropped == 0 {
		return limit
	}

	// Scale the batch to the target at its cost a block, within the bounds.
	elapsed = max(elapsed, time.Millisecond)
	next := int(int64(dropped) * int64(b.WriteTarget) / int64(elapsed))
	next = min(max(next, limit/2), limit*2)
	return b.clamp(next)
}
