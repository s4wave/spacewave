package block

import (
	"context"
	"encoding/binary"
	"math/bits"
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
// one process-wide table of 512 KiB. Cache and pack log construction prepare
// it outside request locks. GoScript would build each slot as an object if
// the table were initialized at package load.
const decodedBlockRemovalStripes = 1 << 16

// decodedBlockRemovalTable holds the generation of the latest removal hashing
// to each stripe.
type decodedBlockRemovalTable [decodedBlockRemovalStripes]atomic.Uint64

// decodedBlockRemovals is the process-wide removal state.
var decodedBlockRemovals struct {
	// gen advances on every removal batch.
	gen atomic.Uint64
	// stripes is initialized by PrepareBlockRemovalTracking or the first removal.
	stripes atomic.Pointer[decodedBlockRemovalTable]
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

// BlockDigestTailLen is the number of trailing bytes a packed block lookup
// uses from the protobuf encoding of its hash. Removal stripes use the same
// identity so every ref that could read that location is invalidated.
const BlockDigestTailLen = 16

// InvalidateRemovedBlock records that a durable store removed ref. Call it
// after the removal is durable, such as after the transaction that deleted the
// block commits. Calling it for a removal that did not happen only costs a miss.
func InvalidateRemovedBlock(ref *BlockRef) {
	if !ref.GetEmpty() {
		InvalidateRemovedBlockDigest(ref.GetHash().MarshalDigest())
	}
}

// InvalidateRemovedBlockDigest records a removal by the encoded hash used in
// a pack index or its BlockDigestTailLen-byte lookup key.
func InvalidateRemovedBlockDigest(digest []byte) {
	InvalidateRemovedBlockStripe(BlockRemovalStripeForDigest(digest))
}

// BlockRemovalStripe identifies a generation slot shared by digest tails.
type BlockRemovalStripe uint16

// BlockRemovalStripeForDigest computes the stripe before the caller takes
// the lock that protects the locations it will remove.
func BlockRemovalStripeForDigest(digest []byte) BlockRemovalStripe {
	return decodedBlockRemovalStripe(string(digest))
}

// InvalidateRemovedBlockStripe publishes a single removal with its precomputed
// stripe, before the caller makes the location unreadable.
func InvalidateRemovedBlockStripe(stripe BlockRemovalStripe) {
	// A single stripe needs no batch of slots.
	stripes := decodedBlockRemovalStripesTable()
	raiseRemovalGeneration(&stripes[stripe], decodedBlockRemovals.gen.Add(1))
}

// BlockRemovalBatch collects the distinct stripes a store is removing. The
// zero value is empty. Publish it before releasing the lock that hides the
// location changes from readers.
type BlockRemovalBatch [decodedBlockRemovalStripes / 64]uint64

// Add marks stripe for invalidation, merging entries that share it.
func (b *BlockRemovalBatch) Add(stripe BlockRemovalStripe) {
	b[stripe/64] |= uint64(1) << (stripe % 64)
}

// Publish assigns one generation to the batch and raises each marked stripe.
// An empty batch leaves the generation unchanged.
func (b *BlockRemovalBatch) Publish() {
	// Reserve a generation only for a nonempty batch.
	var table *decodedBlockRemovalTable
	var gen uint64
	for i, marked := range b {
		if marked == 0 {
			continue
		}
		if table == nil {
			table = decodedBlockRemovalStripesTable()
			gen = decodedBlockRemovals.gen.Add(1)
		}

		// Publish each set bit once, even when several keys mapped to it.
		for ; marked != 0; marked &= marked - 1 {
			stripe := i*64 + bits.TrailingZeros64(marked)
			raiseRemovalGeneration(&table[stripe], gen)
		}
	}
}

// InvalidateRemovedTombstones records each tombstone entry of a batch a
// durable store applied as a removal, as InvalidateRemovedBlock does.
func InvalidateRemovedTombstones(entries []*PutBatchEntry) {
	for _, entry := range entries {
		if entry != nil && entry.Tombstone {
			InvalidateRemovedBlock(entry.Ref)
		}
	}
}

// InvalidateAllRemovedBlocks records that a durable store removed every block
// it held, such as when its volume is deleted. Every entry in the pool
// becomes a miss, in every scope: an entry records the scope that read it,
// not the store that served it, so the pool cannot tell which entries the
// removed store backed. Deleting a store is rare, and the cost is one miss per
// entry.
func InvalidateAllRemovedBlocks() {
	raiseRemovalGeneration(&decodedBlockRemovals.cleared, decodedBlockRemovals.gen.Add(1))
}

// PrepareBlockRemovalTracking allocates the process-wide removal table before
// a store starts recording removals under its lock.
func PrepareBlockRemovalTracking() {
	decodedBlockRemovalStripesTable()
}

// decodedBlockRemovalStripesTable returns the stripe table, allocating it on
// first use by a constructor or a store without a decoded cache.
func decodedBlockRemovalStripesTable() *decodedBlockRemovalTable {
	if stripes := decodedBlockRemovals.stripes.Load(); stripes != nil {
		return stripes
	}
	decodedBlockRemovals.stripes.CompareAndSwap(nil, new(decodedBlockRemovalTable))
	return decodedBlockRemovals.stripes.Load()
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

// RemovedSince reports whether a durable store may have removed ref after
// DurableReadGeneration returned gen. A store that keeps a block's bytes in
// memory past the read that found it checks this before answering from them,
// as the pool does before serving an entry.
func RemovedSince(ref *BlockRef, gen uint64) bool {
	refKey, ok := decodedBlockCacheRefKey(ref)
	return ok && removedSince(refKey, gen)
}

// removedSince reports whether a removal of refKey's stripe or of a whole
// store happened after generation gen.
func removedSince(refKey string, gen uint64) bool {
	return removedStripeSince(decodedBlockRemovalStripe(blockRefDigestKey(refKey)), gen)
}

// removedStripeSince checks a precomputed stripe and whole-store removals.
func removedStripeSince(stripe BlockRemovalStripe, gen uint64) bool {
	if decodedBlockRemovals.cleared.Load() > gen {
		return true
	}
	stripes := decodedBlockRemovals.stripes.Load()
	return stripes != nil && stripes[stripe].Load() > gen
}

// blockRefDigestKey returns the hash field of a marshaled BlockRef. The
// generated marshaler writes field 1 first, followed by any unknown fields.
func blockRefDigestKey(refKey string) string {
	if len(refKey) != 0 && refKey[0] == 0x0a {
		size, n := binary.Uvarint([]byte(refKey[1:]))
		if n > 0 {
			digest := refKey[1+n:]
			if size <= uint64(len(digest)) {
				return digest[:size]
			}
		}
	}
	return refKey
}

// decodedBlockRemovalStripe maps a packed lookup key to its removal stripe.
func decodedBlockRemovalStripe(digest string) BlockRemovalStripe {
	return BlockRemovalStripe(decodedBlockRefHash(digest[max(0, len(digest)-BlockDigestTailLen):]) % decodedBlockRemovalStripes)
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
	// keep stores the block once the reader decoded its refs, set by a store
	// that served it without them.
	keep func(context.Context, []*BlockRef)
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

// KeepDecodedRefs asks the reader of ref on ctx to call keep with a copy of
// data, the block's stored bytes, and the block's refs once it decodes the
// block. A store that served the block without its refs, which it cannot store
// as a leaf, stores it this way with the refs a write of the block records.
// keep runs at most once, after the read returns, and only for a block the
// reader decoded. A read nobody marks drops it without copying data.
func KeepDecodedRefs(ctx context.Context, ref *BlockRef, data []byte, keep func(ctx context.Context, data []byte, refs []*BlockRef)) {
	// Only the reader of this ref can supply its decoded references.
	mark := blockReadMarkFromContext(ctx)
	if mark == nil || !mark.ref.EqualsRef(ref) {
		return
	}

	// Copy the bytes: the reader may decode them in place.
	data = append([]byte(nil), data...)
	mark.mtx.Lock()
	mark.keep = func(ctx context.Context, refs []*BlockRef) { keep(ctx, data, refs) }
	mark.mtx.Unlock()
}

// takeKeep returns and clears the read's keep, or nil.
func (m *blockReadMark) takeKeep() func(context.Context, []*BlockRef) {
	// An untracked read cannot have a callback.
	if m == nil {
		return nil
	}

	// Transfer the callback to its sole consumer.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	keep := m.keep
	m.keep = nil
	return keep
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
