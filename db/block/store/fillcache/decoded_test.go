package block_store_fillcache

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestCacheDecodedPoolAdmission reads a filled block twice through the cache
// with a shared decoded-block cache: the first read admits it to the pool and
// the second decodes it from the pool without a store read. After the cache
// removes the block for a failed hash, and after an eviction step deletes it,
// the next read misses the pool and reads the store again.
func TestCacheDecodedPoolAdmission(t *testing.T) {
	ctx := t.Context()
	c := openTest(t, filepath.Join(t.TempDir(), "cache", "blocks.db"), 64<<20)
	src := newPacks()
	reads := c.Reads(func() block.StoreOps { return src })
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer decodedBlocks.Close()

	// read reads ref through the cache in a new transaction and returns the
	// store reads it took.
	read := func(ref *block.BlockRef) uint64 {
		t.Helper()
		rctx, counter := block.WithReadCounter(ctx)
		tx, cursor := block.NewTransaction(reads(), nil, ref, nil)
		tx.SetDecodedBlockCache(decodedBlocks)
		if _, err := cursor.Unmarshal(rctx, block_mock.NewExampleBlock); err != nil {
			t.Fatal(err)
		}
		return counter.Snapshot().BlockReadCount
	}

	// fill puts a block in the durable store, copies it into the cache, and
	// waits for the copy.
	fill := func(msg string) *block.BlockRef {
		t.Helper()
		ref, _, err := block.PutBlock(ctx, src, &block_mock.Example{Msg: msg})
		if err != nil {
			t.Fatal(err)
		}
		stored, err := src.GetStoredBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Fill(ctx, nil, ref, stored); err != nil {
			t.Fatal(err)
		}
		wait(t, c)
		return ref
	}

	// admitted reads ref twice: the first read reads the cache and stores
	// into the pool, the second hits the pool with no store read.
	admitted := func(ref *block.BlockRef) {
		t.Helper()
		before := decodedBlocks.Snapshot()
		srcReads := src.reads.Load()
		if n := read(ref); n != 1 {
			t.Fatalf("first read took %d store reads, want 1", n)
		}
		if got := src.reads.Load(); got != srcReads {
			t.Fatalf("first read reached the durable store: %d -> %d", srcReads, got)
		}
		mid := decodedBlocks.Snapshot()
		if mid.Stores != before.Stores+1 {
			t.Fatalf("a cache read stored %d blocks into the pool, want 1", mid.Stores-before.Stores)
		}
		if n := read(ref); n != 0 {
			t.Fatalf("second read took %d store reads, want 0", n)
		}
		if after := decodedBlocks.Snapshot(); after.Hits != mid.Hits+1 {
			t.Fatalf("second read missed the pool: hits %d -> %d", mid.Hits, after.Hits)
		}
	}

	// refetched reads ref once and checks it missed the pool and read the
	// durable store.
	refetched := func(ref *block.BlockRef, why string) {
		t.Helper()
		hits := decodedBlocks.Snapshot().Hits
		srcReads := src.reads.Load()
		if n := read(ref); n == 0 {
			t.Fatalf("read after %s took no store read", why)
		}
		if got := decodedBlocks.Snapshot().Hits; got != hits {
			t.Fatalf("read after %s hit the pool: hits %d -> %d", why, hits, got)
		}
		if got := src.reads.Load(); got != srcReads+1 {
			t.Fatalf("read after %s did not reach the durable store: %d -> %d", why, srcReads, got)
		}
	}

	// A removal for a failed hash reports the block removed.
	removed := fill("removed for its hash")
	admitted(removed)
	c.remove(removed)
	wait(t, c)
	refetched(removed, "remove")

	// An eviction step deleting the block reports it removed. A step starts
	// at a random key, so it may delete only the other row first: step until
	// the block is gone.
	evicted := fill("evicted")
	admitted(evicted)
	wait(t, c)
	for i := 0; ; i++ {
		if err := c.evict.step(ctx); err != nil {
			t.Fatal(err)
		}
		held, err := c.blocks.GetBlockExists(ctx, evicted)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			break
		}
		if i == 100 {
			t.Fatal("100 eviction steps left the block in the cache")
		}
	}
	refetched(evicted, "eviction")
}
