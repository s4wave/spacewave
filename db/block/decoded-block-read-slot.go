package block

import "sync/atomic"

// decodedBlockReadSlot is the evictable identity of one pool entry. Links may
// outlive its entry, but eviction atomically drops all decoded data and edges.
type decodedBlockReadSlot struct {
	// data is nil before attachment and after retirement.
	data atomic.Pointer[decodedBlockReadData]
	// recent lets pointer hits participate in the existing pool's eviction policy.
	recent atomic.Bool
}

// decodedBlockReadLink remembers a child's reference and slot without retaining
// its payload. A raw edge, such as a leaf value, has no decoder and so never
// gets a slot: its link memoizes only the reference, so following it again
// allocates nothing.
type decodedBlockReadLink struct {
	// ref is the immutable reference stored in the parent block.
	ref *BlockRef
	// slot is the most recently admitted child entry; nil for a raw edge.
	slot atomic.Pointer[decodedBlockReadSlot]
}

// decodedBlockReadData holds only immutable content and bounded edge metadata.
// Its pages are already charged to the pool; attachment charges its extra heap.
type decodedBlockReadData struct {
	// block is the pool's immutable decoded instance.
	block Block
	// cache owns the entry's exact storage and transform scope.
	cache *DecodedBlockCache
	// refKey identifies the removal and invalidation stripe.
	refKey string
	// epochs is the scope state at durable admission.
	epochs decodedBlockCacheEpochs
	// durableGen is the durable snapshot's removal generation.
	durableGen uint64
	// cost is the pool budget charged for this metadata at attachment.
	cost int64
	// links indexes child references by their immutable field IDs.
	links map[uint32]*decodedBlockReadLink
	// edges owns the contiguous child link allocation.
	edges []decodedBlockReadLink
}

// newDecodedBlockReadData builds child metadata outside every cache shard lock.
// Every nonempty reference gets a link; only a typed one can later hold a slot,
// so a raw one is charged for its reference alone.
func newDecodedBlockReadData(cache *DecodedBlockCache, key decodedBlockCacheKey, entry decodedBlockCacheEntry) (*decodedBlockReadData, int64) {
	// Preserve the exact cache identity and its durable-admission state.
	data := &decodedBlockReadData{block: entry.block, cache: cache, refKey: key.ref, epochs: entry.epochs, durableGen: entry.durableGen}
	cost := int64(128 + len(key.ref))
	withRefs, ok := entry.block.(BlockWithRefs)
	if !ok {
		return data, cost
	}

	// Inspect reference descriptors without loading their target blocks.
	refs, err := withRefs.GetBlockRefs()
	if err != nil {
		return nil, 0
	}
	var count int
	for _, ref := range refs {
		if !ref.GetEmpty() {
			count++
		}
	}
	if count == 0 {
		return data, cost
	}

	// Reserve and charge every memoized edge before attachment.
	data.links = make(map[uint32]*decodedBlockReadLink, count)
	data.edges = make([]decodedBlockReadLink, count)
	index := 0
	var edgeCost int64
	for id, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		edge := &data.edges[index]
		edge.ref = ref.Clone()
		edgeCost += 64 + 2*int64(ref.SizeVT())
		if withRefs.GetBlockRefCtor(id) != nil {
			edgeCost += 192
		}
		data.links[id] = edge
		index++
	}
	return data, cost + edgeCost
}

// readSlot attaches immutable traversal metadata to a currently admitted entry.
// A superseded, evicted, oversized or invalid entry remains on the normal path.
func (c *DecodedBlockCache) readSlot(key decodedBlockCacheKey) *decodedBlockReadSlot {
	// Require an active, valid and read-shareable decoded entry.
	if !c.enabled() {
		return nil
	}
	poolKey := c.cacheKey(key)
	entry, ok := c.pool.get(poolKey)
	if !ok || entry.read == nil || entry.epochs != c.epochs(key.ref) || removedSince(key.ref, entry.durableGen) {
		return nil
	}
	if entry.read.data.Load() != nil {
		return entry.read
	}

	// Build metadata without holding the shard or invoking storage operations.
	data, cost := newDecodedBlockReadData(c, key, entry)
	if data == nil || !c.pool.attachReadData(poolKey, entry.read, data, cost) {
		return nil
	}
	return entry.read
}
