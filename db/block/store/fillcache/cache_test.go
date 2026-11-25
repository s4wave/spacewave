package block_store_fillcache

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// packs stands in for the durable store: an in-memory store of blocks with
// known refs that counts the reads it serves.
type packs struct {
	block.StoreOps
	// reads counts the block reads.
	reads atomic.Int64
}

// newPacks returns an empty durable store stand-in.
func newPacks() *packs {
	return &packs{StoreOps: block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0)}
}

// GetStoredBlock counts the read and serves the block as a leaf with known refs.
func (p *packs) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	p.reads.Add(1)
	stored, err := block.GetBlockWithoutRefs(ctx, p.StoreOps, ref)
	if stored != nil {
		stored.RefsKnown = true
	}
	return stored, err
}

// openTest opens a cache in dir with a size bound, waits for its packs to load,
// and closes it when the test ends.
func openTest(t *testing.T, dir string, limit int64) *Cache {
	t.Helper()
	c, err := Open(t.Context(), logrus.NewEntry(logrus.New()), dir, Options{LimitBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err := c.packs.Ready(t.Context()); err != nil {
		t.Fatal(err)
	}
	return c
}

// testBlocks puts n blocks of 16 KiB in a new durable store and returns it with
// their refs.
func testBlocks(t *testing.T, n int) (*packs, []*block.BlockRef) {
	t.Helper()
	src := newPacks()
	refs := make([]*block.BlockRef, 0, n)
	for i := range n {
		ref, _, err := src.PutBlock(t.Context(), bytes.Repeat([]byte{byte(i), byte(i >> 8)}, 8<<10), &block.PutOpts{})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	return src, refs
}

// fillRead reads ref through c's view, then src, as a placed store reads, and
// waits for the writer.
func fillRead(t *testing.T, c *Cache, src *packs, ref *block.BlockRef) {
	t.Helper()
	reads := block_store.NewStoreReadThrough(c.View, c.Reads(func() block.StoreOps { return src }), nil)
	data, found, err := reads.GetBlock(t.Context(), ref)
	if err != nil || !found || len(data) != 16<<10 {
		t.Fatalf("read = %d bytes, %v, %v", len(data), found, err)
	}
	wait(t, c)
}

// wait waits until the cache's writer is idle.
func wait(t *testing.T, c *Cache) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := c.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestCacheServesFillsAfterReopen fills a cache from the durable store, closes
// it as a restart would, and checks that the reopened cache serves each block
// with its refs without reading the durable store, while a block it never
// took reads as a miss.
func TestCacheServesFillsAfterReopen(t *testing.T) {
	// Fill three of four blocks.
	dir := t.TempDir()
	c := openTest(t, dir, 1<<30)
	src, refs := testBlocks(t, 4)
	for _, ref := range refs[:3] {
		fillRead(t, c, src, ref)
	}
	if counts := c.Counts(); counts.Written != 3 || counts.Misses != 3 {
		t.Fatalf("counts after the fills = %+v, want 3 written and 3 misses", counts)
	}
	c.Close()

	// The reopened cache serves the three from its packs alone.
	c = openTest(t, dir, 1<<30)
	src.reads.Store(0)
	for i, ref := range refs {
		stored, err := c.View().GetStoredBlock(t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			if stored != nil {
				t.Fatal("the reopened cache served a block it never took")
			}
			continue
		}
		if stored == nil || len(stored.Data) != 16<<10 || !stored.RefsKnown {
			t.Fatalf("block %d after reopen = %+v, want its data with known refs", i, stored)
		}
	}
	if n := src.reads.Load(); n != 0 {
		t.Fatalf("the reopened cache read the durable store %d times", n)
	}
}
