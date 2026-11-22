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
// known refs, or unknown refs when refsUnknown is set, that counts the reads it
// serves.
type packs struct {
	block.StoreOps
	// reads counts the block reads.
	reads atomic.Int64
	// refsUnknown serves every block with its refs unknown, as a pack holds a
	// block written without them.
	refsUnknown bool
}

// newPacks returns an empty durable store stand-in.
func newPacks() *packs {
	return &packs{StoreOps: block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)}
}

// GetStoredBlock counts the read and serves the block as a leaf with known
// refs, or with unknown refs when refsUnknown is set.
func (p *packs) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	p.reads.Add(1)
	stored, err := block.GetBlockWithoutRefs(ctx, p.StoreOps, ref)
	if stored != nil {
		stored.RefsKnown = !p.refsUnknown
	}
	return stored, err
}

// openTest opens a cache at path with a size bound, closed when the test ends.
func openTest(t *testing.T, path string, limit int64) *Cache {
	t.Helper()
	return openTestOpts(t, path, Options{LimitBytes: limit})
}

// openTestOpts opens a cache at path with opts, closed when the test ends.
func openTestOpts(t *testing.T, path string, opts Options) *Cache {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	c, err := Open(t.Context(), le, path, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
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

// fillRead reads ref through c from src and waits for the writer.
func fillRead(t *testing.T, c *Cache, src *packs, ref *block.BlockRef) {
	t.Helper()
	data, found, err := c.Reads(func() block.StoreOps { return src })().GetBlock(t.Context(), ref)
	if err != nil || !found || len(data) != 16<<10 {
		t.Fatalf("read = %d bytes, %v, %v", len(data), found, err)
	}
	wait(t, c)
}

// holds reports whether the cache's table holds ref's row, without a read
// that would stamp it.
func holds(t *testing.T, c *Cache, ref *block.BlockRef) bool {
	t.Helper()
	key, err := c.evict.blockKey(ref)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := c.store.GetDB().QueryRowContext(t.Context(), "SELECT count(*) FROM "+cacheTable+" WHERE key = ?", key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// closeTestWindow closes c's start window now, as its timer would, and waits
// for the writer to store the range.
func closeTestWindow(t *testing.T, c *Cache) {
	t.Helper()
	c.closeWindow()
	select {
	case <-c.windowDone:
	case <-time.After(20 * time.Second):
		t.Fatal("the start window was not stored")
	}
	wait(t, c)
}

// storedRange returns the start range c's file holds.
func storedRange(t *testing.T, c *Cache) (int64, int64, bool) {
	t.Helper()
	lo, hi, ok, err := startRange(t.Context(), c.store.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	return lo, hi, ok
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
	if got, want := c.Counts(), (Counts{Hits: 1, Misses: 1, Queued: 1, Written: 1}); got != want {
		t.Fatalf("counts after a fill and a hit = %+v, want %+v", got, want)
	}

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
	if counts := c.Counts(); counts.Evicted == 0 || counts.Written < counts.Evicted+uint64(rows) {
		t.Fatalf("counts after eviction = %+v with %d rows left", counts, rows)
	}
	for i := range refs {
		read(i)
	}
}

// TestCacheFillsBlocksWithUnknownRefs reads a block the durable store serves
// with its refs unknown, as most pack blocks are: the read fills the cache,
// the next read is served from it without touching the durable store, and the
// cached block still reports its refs unknown.
func TestCacheFillsBlocksWithUnknownRefs(t *testing.T) {
	// Put a block in a durable store that serves it with unknown refs.
	src := newPacks()
	src.refsUnknown = true
	data := bytes.Repeat([]byte("unknown refs"), 1024)
	ref, _, err := src.PutBlock(t.Context(), data, &block.PutOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Read the block through the cache and wait for its fill.
	c := openTest(t, filepath.Join(t.TempDir(), "blocks.db"), 0)
	reads := c.Reads(func() block.StoreOps { return src })()
	if got, found, err := reads.GetBlock(t.Context(), ref); err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("first read = %d bytes, %v, %v", len(got), found, err)
	}
	wait(t, c)

	// Read it again: the cache serves it, with its refs still unknown.
	before := src.reads.Load()
	stored, err := reads.GetStoredBlock(t.Context(), ref)
	if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("second read = %v, %v", stored, err)
	}
	if n := src.reads.Load() - before; n != 0 {
		t.Fatalf("second read touched the durable store %d times, want 0", n)
	}
	if stored.RefsKnown {
		t.Fatal("the cached block reports its refs known")
	}
}

// TestCacheEvictionKeepsWhatAStartRead fills a cache, restarts it, reads part
// of it again as a start does, then fills new blocks until an eviction cycle
// runs: the cycle deletes the rows not read since the restart and keeps every
// block the start read.
func TestCacheEvictionKeepsWhatAStartRead(t *testing.T) {
	// Put blocks in the durable store.
	path := filepath.Join(t.TempDir(), "blocks.db")
	const limit = 1 << 20
	src := newPacks()
	var refs []*block.BlockRef
	for i := range 100 {
		ref, _, err := src.PutBlock(t.Context(), bytes.Repeat([]byte{byte(i)}, 16<<10), &block.PutOpts{})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}

	// read reads block i through c and reports whether the cache served it.
	read := func(c *Cache, i int) bool {
		t.Helper()
		before := src.reads.Load()
		data, found, err := c.Reads(func() block.StoreOps { return src })().GetBlock(t.Context(), refs[i])
		if err != nil || !found || len(data) != 16<<10 {
			t.Fatalf("read %d = %d bytes, %v, %v", i, len(data), found, err)
		}
		wait(t, c)
		return src.reads.Load() == before
	}

	// Fill the first blocks, short of the bound, and restart.
	const filled, started = 45, 25
	c := openTest(t, path, limit)
	for i := range filled {
		read(c, i)
	}
	if n := c.Counts().Evicted; n != 0 {
		t.Fatalf("filling %d blocks evicted %d rows; the test needs them under the bound", filled, n)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c = openTest(t, path, limit)

	// The start reads the first blocks, which the cache serves.
	for i := range started {
		if !read(c, i) {
			t.Fatalf("block %d was not cached after the restart", i)
		}
	}

	// New fills run an eviction cycle, which takes only rows the start did
	// not read.
	next := filled
	for c.Counts().Evicted < 4 {
		if next == len(refs) {
			t.Fatalf("no eviction after %d fills", next-filled)
		}
		read(c, next)
		next++
	}
	if n := c.Counts().Evicted; n > filled-started {
		t.Fatalf("one cycle evicted %d rows, more than the %d the start did not read", n, filled-started)
	}
	for i := range started {
		if !read(c, i) {
			t.Fatalf("block %d, read by the start, was evicted", i)
		}
	}
}

// TestCacheEvictionSparesTheStartWindow reads a start's blocks inside its
// window, closes the window, then fills later blocks past the bound: eviction
// deletes later rows and keeps every row the window stamped, though none of
// them was read again.
func TestCacheEvictionSparesTheStartWindow(t *testing.T) {
	const limit, started = 1 << 20, 20
	src, refs := testBlocks(t, 120)
	c := openTestOpts(t, filepath.Join(t.TempDir(), "blocks.db"), Options{LimitBytes: limit, StartWindow: time.Hour})

	// The start reads its blocks, then its window closes.
	for i := range started {
		fillRead(t, c, src, refs[i])
	}
	closeTestWindow(t, c)
	if lo, hi, ok := storedRange(t, c); !ok || lo != c.startSeq || hi != c.seq-1 {
		t.Fatalf("stored start range = %d..%d %v, want %d..%d", lo, hi, ok, c.startSeq, c.seq-1)
	}

	// Later fills push the cache past its bound several times over.
	for i := started; i < len(refs); i++ {
		fillRead(t, c, src, refs[i])
	}
	if c.Counts().Evicted < uint64(len(refs)-started)/2 {
		t.Fatalf("later fills evicted %d rows; the test needs many cycles", c.Counts().Evicted)
	}
	for i := range started {
		if !holds(t, c, refs[i]) {
			t.Fatalf("block %d, stamped in the start window, was evicted", i)
		}
	}
}

// TestCacheStartRangeSurvivesRestart stores a start range, restarts, and fills
// past the bound inside the next start's window: the next process spares the
// stored range. A third process whose short window closes with nothing
// stamped keeps the range in place.
func TestCacheStartRangeSurvivesRestart(t *testing.T) {
	const limit, started = 1 << 20, 20
	path := filepath.Join(t.TempDir(), "blocks.db")
	src, refs := testBlocks(t, 120)

	// The first process reads its start's blocks and closes its window.
	c := openTestOpts(t, path, Options{LimitBytes: limit, StartWindow: time.Hour})
	for i := range started {
		fillRead(t, c, src, refs[i])
	}
	closeTestWindow(t, c)
	lo, hi, _ := storedRange(t, c)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	// The next process, inside its own window, fills past the bound and
	// spares the stored range.
	c = openTestOpts(t, path, Options{LimitBytes: limit, StartWindow: time.Hour})
	if c.evict.lo != lo || c.evict.hi != hi {
		t.Fatalf("reopened cache protects %d..%d, want %d..%d", c.evict.lo, c.evict.hi, lo, hi)
	}
	for i := started; i < len(refs); i++ {
		fillRead(t, c, src, refs[i])
	}
	if c.Counts().Evicted == 0 {
		t.Fatal("later fills evicted nothing; the test needs eviction")
	}
	for i := range started {
		if !holds(t, c, refs[i]) {
			t.Fatalf("block %d, in the stored start range, was evicted after the restart", i)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	// A short window that stamps nothing closes on its timer and keeps the
	// range.
	c = openTestOpts(t, path, Options{LimitBytes: limit, StartWindow: 10 * time.Millisecond})
	select {
	case <-c.windowDone:
	case <-time.After(20 * time.Second):
		t.Fatal("the start window timer never closed the window")
	}
	if l, h, ok := storedRange(t, c); !ok || l != lo || h != hi {
		t.Fatalf("an empty window left %d..%d %v, want %d..%d", l, h, ok, lo, hi)
	}
}

// TestCacheEvictsAStartRangeLargerThanTheBound stores a start range whose rows
// alone exceed the bound, then reopens the cache with that bound: eviction
// deletes inside the range once nothing else is left, and the cache reaches
// its target instead of stalling.
func TestCacheEvictsAStartRangeLargerThanTheBound(t *testing.T) {
	const limit = 512 << 10
	path := filepath.Join(t.TempDir(), "blocks.db")
	src, refs := testBlocks(t, 61)

	// A larger cache stores a start range of 60 blocks, about 1 MiB.
	c := openTestOpts(t, path, Options{LimitBytes: 8 << 20, StartWindow: time.Hour})
	for i := range 60 {
		fillRead(t, c, src, refs[i])
	}
	closeTestWindow(t, c)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopened under a smaller bound, one fill starts an eviction that must
	// go into the range.
	c = openTestOpts(t, path, Options{LimitBytes: limit, StartWindow: time.Hour})
	fillRead(t, c, src, refs[60])
	size, err := c.Size(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if size > c.evict.target {
		t.Fatalf("cache holds %d bytes after eviction, want at most %d", size, c.evict.target)
	}
	if holds(t, c, refs[60]) {
		t.Fatal("the fill outside the range survived while range rows were evicted")
	}
	var kept int
	for i := range 60 {
		if holds(t, c, refs[i]) {
			kept++
		}
	}
	if kept == 0 || kept == 60 {
		t.Fatalf("eviction kept %d of the 60 range rows", kept)
	}
}
