package block_store_fillcache

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/sirupsen/logrus"
)

// keptPacks is a durable store that declares it never removes a block.
type keptPacks struct {
	*packs
}

// KeepsBlocks reports that the store never removes a block.
func (keptPacks) KeepsBlocks() bool { return true }

// decodedHarness reads blocks through a cache over a durable store with a
// shared decoded-block cache.
type decodedHarness struct {
	t *testing.T
	// src is the durable store's counted reads.
	src *packs
	// lower is the durable store the cache reads through.
	lower block.StoreOps
	// pool is the shared decoded-block cache.
	pool *block.DecodedBlockCache
}

// newDecodedHarness builds a harness over src, read as lower.
func newDecodedHarness(t *testing.T, src *packs, lower block.StoreOps) *decodedHarness {
	pool, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &decodedHarness{t: t, src: src, lower: lower, pool: pool}
}

// read reads ref through c in a new transaction and returns the store reads
// it took.
func (h *decodedHarness) read(c *Cache, ref *block.BlockRef) uint64 {
	h.t.Helper()
	rctx, counter := block.WithReadCounter(h.t.Context())
	tx, cursor := block.NewTransaction(c.Reads(func() block.StoreOps { return h.lower })(), nil, ref, nil)
	tx.SetDecodedBlockCache(h.pool)
	if _, err := cursor.Unmarshal(rctx, block_mock.NewExampleBlock); err != nil {
		h.t.Fatal(err)
	}
	return counter.Snapshot().BlockReadCount
}

// fill puts a block in the durable store, copies it into c, and waits for the
// copy.
func (h *decodedHarness) fill(c *Cache, msg string) *block.BlockRef {
	h.t.Helper()
	ctx := h.t.Context()
	ref, _, err := block.PutBlock(ctx, h.src, &block_mock.Example{Msg: msg})
	if err != nil {
		h.t.Fatal(err)
	}
	stored, err := h.src.GetStoredBlock(ctx, ref)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := c.Fill(ctx, nil, ref, stored); err != nil {
		h.t.Fatal(err)
	}
	wait(h.t, c)
	return ref
}

// admitted reads ref twice: the first read reads the cache and stores into
// the pool, the second hits the pool with no store read.
func (h *decodedHarness) admitted(c *Cache, ref *block.BlockRef) {
	h.t.Helper()
	before := h.pool.Snapshot()
	srcReads := h.src.reads.Load()
	if n := h.read(c, ref); n != 1 {
		h.t.Fatalf("first read took %d store reads, want 1", n)
	}
	if got := h.src.reads.Load(); got != srcReads {
		h.t.Fatalf("first read reached the durable store: %d -> %d", srcReads, got)
	}
	mid := h.pool.Snapshot()
	if mid.Stores != before.Stores+1 {
		h.t.Fatalf("a cache read stored %d blocks into the pool, want 1", mid.Stores-before.Stores)
	}
	if n := h.read(c, ref); n != 0 {
		h.t.Fatalf("second read took %d store reads, want 0", n)
	}
	if after := h.pool.Snapshot(); after.Hits != mid.Hits+1 {
		h.t.Fatalf("second read missed the pool: hits %d -> %d", mid.Hits, after.Hits)
	}
}

// missed reads ref once and checks it missed the pool.
func (h *decodedHarness) missed(c *Cache, ref *block.BlockRef, why string) {
	h.t.Helper()
	hits := h.pool.Snapshot().Hits
	if n := h.read(c, ref); n == 0 {
		h.t.Fatalf("read after %s took no store read", why)
	}
	if got := h.pool.Snapshot().Hits; got != hits {
		h.t.Fatalf("read after %s hit the pool: hits %d -> %d", why, hits, got)
	}
}

// refetched reads ref once and checks it missed the pool and read the durable
// store.
func (h *decodedHarness) refetched(c *Cache, ref *block.BlockRef, why string) {
	h.t.Helper()
	srcReads := h.src.reads.Load()
	h.missed(c, ref, why)
	if got := h.src.reads.Load(); got != srcReads+1 {
		h.t.Fatalf("read after %s did not reach the durable store: %d -> %d", why, srcReads, got)
	}
}

// TestCacheDecodedPoolAdmission reads a filled block twice through the cache
// over a store that keeps its blocks, with a shared decoded-block cache: the
// first read admits it to the pool and the second decodes it from the pool
// without a store read. After the cache removes the block for a failed hash,
// the next read misses the pool and reads the store again. An eviction step
// reports the block removed before its delete, so a read once the row is gone
// misses the pool.
func TestCacheDecodedPoolAdmission(t *testing.T) {
	ctx := t.Context()
	c := openTest(t, filepath.Join(t.TempDir(), "cache", "blocks.db"), 64<<20)
	src := newPacks()
	h := newDecodedHarness(t, src, keptPacks{src})

	// A removal for a failed hash reports the block removed.
	removed := h.fill(c, "removed for its hash")
	h.admitted(c, removed)
	c.remove(removed)
	wait(t, c)
	h.refetched(c, removed, "remove")

	// A step starts at a random key, so it may delete only the other row
	// first: step until a delete takes the block, and read it right after that
	// delete, before the step's last report.
	evicted := h.fill(c, "evicted")
	h.admitted(c, evicted)
	wait(t, c)
	var checked bool
	evictDeleted = func() {
		held, err := c.blocks.GetBlockExists(ctx, evicted)
		if err != nil {
			t.Fatal(err)
		}
		if !held && !checked {
			checked = true
			h.refetched(c, evicted, "an eviction delete")
		}
	}
	defer func() { evictDeleted = nil }()
	for i := 0; !checked; i++ {
		if err := c.evict.step(ctx); err != nil {
			t.Fatal(err)
		}
		if i == 100 {
			t.Fatal("100 eviction steps left the block in the cache")
		}
	}
}

// TestCacheNotAdmittedOverRemovingStore reads a filled block through the cache
// over a durable store that does not declare it keeps its blocks. Such a store
// may remove the block, as reclaim does, while the cache goes on serving its
// copy, so no read through the cache may admit the block to the pool.
func TestCacheNotAdmittedOverRemovingStore(t *testing.T) {
	c := openTest(t, filepath.Join(t.TempDir(), "cache", "blocks.db"), 64<<20)
	src := newPacks()
	h := newDecodedHarness(t, src, src)
	ref := h.fill(c, "read over a store that removes")
	for i := range 2 {
		before := h.pool.Snapshot()
		h.missed(c, ref, "a fill")
		if after := h.pool.Snapshot(); after.Stores != before.Stores {
			t.Fatalf("read %d stored the cache's copy into the pool", i)
		}
	}
}

// TestCacheReopenInvalidates admits a block from a cache, closes it, deletes
// its file, and opens a new empty cache on the path. The pool may not serve
// the block the old file held: the next read misses it.
func TestCacheReopenInvalidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "blocks.db")
	c, err := Open(t.Context(), logrus.NewEntry(logrus.New()), path, Options{LimitBytes: 64 << 20})
	if err != nil {
		t.Fatal(err)
	}
	src := newPacks()
	h := newDecodedHarness(t, src, keptPacks{src})
	ref := h.fill(c, "held by the deleted file")
	h.admitted(c, ref)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := removeFiles(path); err != nil {
		t.Fatal(err)
	}
	reopened := openTest(t, path, 64<<20)
	h.refetched(reopened, ref, "a reopen over a deleted file")
}
