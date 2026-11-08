// Package block_store_memcache holds copies of blocks a durable store holds in
// a bounded amount of memory, as a fill cache that writes nothing to disk.
//
// A read through the cache serves a block the cache holds, else reads the
// durable store and keeps a copy of the block, evicting the least recently
// read blocks past the bound. Losing the cache, as a restart does, loses
// nothing: every block in it came from a read of the durable store.
package block_store_memcache

import (
	"container/list"
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// DefaultLimitBytes is the bound of a cache built with no bound.
const DefaultLimitBytes = 1 << 30

// entryOverhead is the memory an entry takes beside its block's bytes: its
// map slot, list element, key and refs header. It keeps a cache of many tiny
// blocks inside its bound too.
const entryOverhead = 256

// Stats counts what a Cache holds and has served.
type Stats struct {
	// Blocks counts the blocks held.
	Blocks int
	// Bytes is the memory they take, as the bound counts it.
	Bytes int64
	// Hits counts the reads the cache served.
	Hits uint64
	// Fills counts the blocks copied in.
	Fills uint64
	// Evicted counts the blocks evicted for the bound.
	Evicted uint64
}

// Cache is a bounded, least recently read set of copies of blocks a durable
// store holds.
//
// Invariant: the cache holds only copies of blocks the durable store holds.
// Fill takes a block only from a read of that store, through Reads, so a block
// only another source holds never enters it. Nothing in it is written to disk.
type Cache struct {
	// limit bounds bytes.
	limit int64
	// hits counts the reads served.
	hits atomic.Uint64
	// fills counts the blocks copied in.
	fills atomic.Uint64
	// evicted counts the blocks evicted.
	evicted atomic.Uint64

	// mtx guards the fields below.
	mtx sync.Mutex
	// entries maps a block's hash to its element in order.
	entries map[string]*list.Element
	// order holds the entries, most recently read first.
	order list.List
	// bytes is the memory the entries take.
	bytes int64
}

// entry is one block held.
type entry struct {
	// key is the block's hash, its key in entries.
	key string
	// stored is the block's copy.
	stored *block.StoredBlock
	// size is the memory the entry counts against the bound.
	size int64
}

// New builds a cache bounded at limitBytes; zero or less uses
// DefaultLimitBytes.
func New(limitBytes int64) *Cache {
	if limitBytes <= 0 {
		limitBytes = DefaultLimitBytes
	}
	return &Cache{limit: limitBytes, entries: make(map[string]*list.Element)}
}

// Reads returns a source that reads the cache, then the durable store lower,
// and keeps a copy of each of lower's hits. A nil lower yields a nil source, as
// no store holds the blocks to copy.
func (c *Cache) Reads(lower block_store.StoreSource) block_store.StoreSource {
	view := &view{cache: c}
	return func() block.StoreOps {
		store := lower()
		if store == nil {
			return nil
		}
		return block_store.NewStoreReadThrough(
			func() block.StoreOps { return view },
			func() block.StoreOps { return store },
			c.Fill,
		)
	}
}

// Fill keeps a copy of a block read from the durable store, evicting the least
// recently read blocks past the bound. It matches block_store.Fill and ignores
// the store it is given.
func (c *Cache) Fill(_ context.Context, _ block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
	// Copy the block, so no reader's change to its slices reaches the cache.
	key := cacheKey(ref)
	copied := &block.StoredBlock{Data: slices.Clone(stored.Data), Refs: stored.Refs, RefsKnown: stored.RefsKnown}
	size := int64(len(copied.Data)) + entryOverhead
	if size > c.limit {
		return nil
	}

	// Keep it as the most recently read, unless it is held already.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		return nil
	}
	c.entries[key] = c.order.PushFront(&entry{key: key, stored: copied, size: size})
	c.bytes += size
	c.fills.Add(1)

	// Evict from the back past the bound.
	for c.bytes > c.limit {
		back := c.order.Back()
		old := back.Value.(*entry)
		c.order.Remove(back)
		delete(c.entries, old.key)
		c.bytes -= old.size
		c.evicted.Add(1)
	}
	return nil
}

// get returns a copy of the block held under ref, and marks it read.
func (c *Cache) get(ref *block.BlockRef) *block.StoredBlock {
	// Find it and mark it.
	c.mtx.Lock()
	el, ok := c.entries[cacheKey(ref)]
	if !ok {
		c.mtx.Unlock()
		return nil
	}
	c.order.MoveToFront(el)
	stored := el.Value.(*entry).stored
	c.mtx.Unlock()

	// Hand out a copy of the bytes, which the reader may change.
	c.hits.Add(1)
	return &block.StoredBlock{Data: slices.Clone(stored.Data), Refs: stored.Refs, RefsKnown: stored.RefsKnown}
}

// Stats returns the counts.
func (c *Cache) Stats() Stats {
	c.mtx.Lock()
	blocks, bytes := len(c.entries), c.bytes
	c.mtx.Unlock()
	return Stats{Blocks: blocks, Bytes: bytes, Hits: c.hits.Load(), Fills: c.fills.Load(), Evicted: c.evicted.Load()}
}

// cacheKey keys a block by its hash.
func cacheKey(ref *block.BlockRef) string {
	return string(ref.GetHash().GetHash())
}
