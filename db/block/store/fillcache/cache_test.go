package block_store_fillcache

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
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
	return &packs{StoreOps: block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)}
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

// openTest opens a cache at path with a size bound, closed when the test ends.
func openTest(t *testing.T, path string, limit int64) *Cache {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	c, err := Open(t.Context(), le, path, Options{LimitBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
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

// TestCacheFillsServesRecreatesAndEvicts walks a cache through its states: a
// read fills it from the durable store and the next read is served from it, a
// block that fails its hash is removed and read again from the durable store,
// a deleted file and a corrupt file are recreated, and rows past the size bound
// are evicted without a read failing.
func TestCacheFillsServesRecreatesAndEvicts(t *testing.T) {
	// Put blocks in the durable store.
	path := filepath.Join(t.TempDir(), "cache", "blocks.db")
	const limit = 512 << 10
	src := newPacks()
	var refs []*block.BlockRef
	for i := range 100 {
		data := bytes.Repeat([]byte{byte(i)}, 16<<10)
		ref, _, err := src.PutBlock(t.Context(), data, &block.PutOpts{})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	want := func(i int) []byte { return bytes.Repeat([]byte{byte(i)}, 16<<10) }

	// Read through the cache, as a placed store does.
	c := openTest(t, path, limit)
	reads := c.Reads(func() block.StoreOps { return src })()
	read := func(i int) {
		t.Helper()
		data, found, err := reads.GetBlock(t.Context(), refs[i])
		if err != nil || !found || !bytes.Equal(data, want(i)) {
			t.Fatalf("read %d = %d bytes, %v, %v", i, len(data), found, err)
		}
	}

	// A read fills the cache from the durable store, and the next one does not
	// touch it.
	fills := func(i int) {
		t.Helper()
		before := src.reads.Load()
		read(i)
		if n := src.reads.Load() - before; n != 1 {
			t.Fatalf("read %d touched the durable store %d times, want 1", i, n)
		}
		wait(t, c)
	}
	cached := func(i int) {
		t.Helper()
		before := src.reads.Load()
		read(i)
		if n := src.reads.Load() - before; n != 0 {
			t.Fatalf("cached read %d touched the durable store %d times", i, n)
		}
	}
	fills(0)
	cached(0)

	// A block that fails its hash reads from the durable store and is removed,
	// then cached again by the read that follows.
	if _, err := c.store.GetDB().ExecContext(t.Context(), "UPDATE "+cacheTable+" SET value = randomblob(16384)"); err != nil {
		t.Fatal(err)
	}
	before := src.reads.Load()
	read(0)
	wait(t, c)
	if src.reads.Load() == before {
		t.Fatal("corrupt block did not read from the durable store")
	}
	read(0)
	wait(t, c)
	cached(0)

	// A file deleted while the cache is stopped is recreated empty.
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := removeFiles(path); err != nil {
		t.Fatal(err)
	}
	c = openTest(t, path, limit)
	reads = c.Reads(func() block.StoreOps { return src })()
	fills(0)
	cached(0)

	// A corrupt file is replaced.
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("not a database "), 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	c = openTest(t, path, limit)
	reads = c.Reads(func() block.StoreOps { return src })()
	fills(1)
	cached(1)

	// Rows past the bound are evicted, and every block still reads.
	for i := range refs {
		read(i)
	}
	wait(t, c)
	size, err := c.Size(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if size > limit {
		t.Fatalf("cache holds %d bytes after eviction, want at most %d", size, limit)
	}
	var rows int
	if err := c.store.GetDB().QueryRowContext(t.Context(), "SELECT count(*) FROM "+cacheTable).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows == 0 || rows >= len(refs) {
		t.Fatalf("cache holds %d of %d blocks after eviction", rows, len(refs))
	}
	for i := range refs {
		read(i)
	}
}
