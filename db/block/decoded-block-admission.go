package block

import (
	"context"
	"sync"
	"sync/atomic"
)

// Invariant: the decoded-block pool shared across transactions holds a block
// only while some durable store holds that block.
//
// Admission is opt-in. A store whose bytes are committed, such as a volume's
// store read outside any write transaction, marks the read durable with
// MarkDurableRead. Every other source, a write buffer, a publication being
// prepared, a write transaction, or a store nobody taught to mark, leaves the
// read unmarked, and its decoded block stays in the read operation's front
// cache. A store that forgets to mark costs hit rate, never correctness.
//
// Removal is the other half. A store that marks reads durable must call
// InvalidateRemovedBlock after each removal it makes durable. The removal
// advances a process-wide generation and records it in the ref's stripe. A
// durable read records the generation current before its snapshot opened, and
// the pool treats an entry whose stripe has moved past that generation as a
// miss, in every scope. A read from a snapshot that predates a removal is
// therefore never admitted after the removal, and an entry admitted before
// the removal is never served after it.

// decodedBlockRemovalStripes is the number of removal generations. A removal
// invalidates every entry whose ref hashes to its stripe, so the table is
// large enough that a sweep of many blocks leaves most entries current. It is
// one process-wide array of 512 KiB.
const decodedBlockRemovalStripes = 1 << 16

// decodedBlockRemovals is the process-wide removal state.
var decodedBlockRemovals struct {
	// gen advances on every removal.
	gen atomic.Uint64
	// stripes hold the generation of the latest removal hashing to each.
	stripes [decodedBlockRemovalStripes]atomic.Uint64
	// cleared holds the generation of the latest removal of a whole store.
	cleared atomic.Uint64
}

// DurableReadGeneration returns the removal generation a durable store records
// before it opens the snapshot that serves a read, and later passes to
// MarkDurableRead. Reading it after the snapshot opens could admit a block a
// removal deleted in between.
func DurableReadGeneration() uint64 {
	return decodedBlockRemovals.gen.Load()
}

// InvalidateRemovedBlock records that a durable store removed ref. Call it
// after the removal is durable, such as after the transaction that deleted the
// block commits; calling it for a removal that did not happen only costs a
// miss.
func InvalidateRemovedBlock(ref *BlockRef) {
	refKey, ok := decodedBlockCacheRefKey(ref)
	if !ok {
		return
	}
	invalidateRemovedRefKey(refKey)
}

// InvalidateAllRemovedBlocks records that a durable store removed every block
// it held, such as when its volume is deleted. Every entry in the pool
// becomes a miss.
func InvalidateAllRemovedBlocks() {
	raiseRemovalGeneration(&decodedBlockRemovals.cleared, decodedBlockRemovals.gen.Add(1))
}

// invalidateRemovedRefKey advances the removal generation and raises refKey's
// stripe to it.
func invalidateRemovedRefKey(refKey string) {
	gen := decodedBlockRemovals.gen.Add(1)
	raiseRemovalGeneration(&decodedBlockRemovals.stripes[decodedBlockRefHash(refKey)%decodedBlockRemovalStripes], gen)
}

// raiseRemovalGeneration raises slot to gen unless it already holds a later
// generation.
func raiseRemovalGeneration(slot *atomic.Uint64, gen uint64) {
	for {
		cur := slot.Load()
		if cur >= gen || slot.CompareAndSwap(cur, gen) {
			return
		}
	}
}

// removedSince reports whether a removal of refKey's stripe or of a whole
// store happened after generation gen.
func removedSince(refKey string, gen uint64) bool {
	return decodedBlockRemovals.cleared.Load() > gen ||
		decodedBlockRemovals.stripes[decodedBlockRefHash(refKey)%decodedBlockRemovalStripes].Load() > gen
}

// decodedBlockRefHash hashes a ref key with unseeded FNV-1a. Ref keys encode
// content hashes, so it spreads them evenly; a collision only invalidates
// more entries than needed.
func decodedBlockRefHash(refKey string) uint64 {
	h := uint64(14695981039346656037)
	for i := range len(refKey) {
		h ^= uint64(refKey[i])
		h *= 1099511628211
	}
	return h
}

// blockReadContextKey is the context key of a blockReadMark.
type blockReadContextKey struct{}

// blockReadMark records where the bytes of one block read came from.
type blockReadMark struct {
	// ref is the block being read. Durable marks for other refs are ignored.
	ref *BlockRef
	// mtx guards the fields below.
	mtx sync.Mutex
	// durable is set when a durable store served ref.
	durable bool
	// gen is the oldest removal generation a durable read of ref recorded.
	gen uint64
	// staged is set when any store served staged content during the read.
	staged bool
}

// withBlockReadMark attaches a fresh mark for a read of ref to ctx.
func withBlockReadMark(ctx context.Context, ref *BlockRef) (context.Context, *blockReadMark) {
	mark := &blockReadMark{ref: ref}
	return context.WithValue(ctx, blockReadContextKey{}, mark), mark
}

// blockReadMarkFromContext returns the read mark on ctx, or nil.
func blockReadMarkFromContext(ctx context.Context) *blockReadMark {
	if ctx == nil {
		return nil
	}
	mark, _ := ctx.Value(blockReadContextKey{}).(*blockReadMark)
	return mark
}

// MarkDurableRead records on ctx that a durable store served ref from a
// snapshot opened after DurableReadGeneration returned gen. Only a store whose
// served bytes are committed may call it, and only for a block it found; a
// store that calls it must also report its removals with
// InvalidateRemovedBlock.
func MarkDurableRead(ctx context.Context, ref *BlockRef, gen uint64) {
	// Ignore reads nobody tracks and durable reads of other blocks.
	mark := blockReadMarkFromContext(ctx)
	if mark == nil || !mark.ref.EqualsRef(ref) {
		return
	}

	// Keep the oldest generation when several durable reads serve the block.
	mark.mtx.Lock()
	if !mark.durable || gen < mark.gen {
		mark.gen = gen
	}
	mark.durable = true
	mark.mtx.Unlock()
}

// MarkStagedRead records on ctx that the block being read came from staged
// content: a write buffer, a publication being prepared, or any other bytes
// that are not yet durably stored. It vetoes admission even when a durable
// store also marked the read.
func MarkStagedRead(ctx context.Context) {
	if mark := blockReadMarkFromContext(ctx); mark != nil {
		mark.mtx.Lock()
		mark.staged = true
		mark.mtx.Unlock()
	}
}

// admission returns the removal generation the read's block may enter the
// shared pool at, and false when it may not: no durable store served it, or a
// store served staged content.
func (m *blockReadMark) admission() (uint64, bool) {
	if m == nil {
		return 0, false
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.gen, m.durable && !m.staged
}
