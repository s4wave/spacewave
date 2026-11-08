package provider_local

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_fillcache "github.com/s4wave/spacewave/db/block/store/fillcache"
	block_store_memcache "github.com/s4wave/spacewave/db/block/store/memcache"
	"github.com/sirupsen/logrus"
)

// TestFillCacheTakesPackHits reads blocks the pack store holds, a block only
// the fallback holds, and one the disk holds, through the disk fill cache and
// the memory one. A pack hit fills the fill cache and not the disk, and the
// next read is served from the cache without the pack store. A fallback hit
// fills the disk with its upload marker and never the cache. A disk cache that
// fails reads as a miss, so the read goes on to the pack store.
func TestFillCacheTakesPackHits(t *testing.T) {
	t.Run("disk", func(t *testing.T) { testFillCacheTakesPackHits(t, false) })
	t.Run("memory", func(t *testing.T) { testFillCacheTakesPackHits(t, true) })
}

// testFillCacheTakesPackHits runs TestFillCacheTakesPackHits on the disk fill
// cache, or the memory one.
func testFillCacheTakesPackHits(t *testing.T, memory bool) {
	// Open the placed store and a fill cache, and count the pack store's reads.
	f := newFallbackTest(t)
	var cache FillCache
	var disk *block_store_fillcache.Cache
	if memory {
		cache = block_store_memcache.New(0)
	} else {
		var err error
		disk, err = block_store_fillcache.Open(f.ctx, logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "cache.db"), block_store_fillcache.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = disk.Close() })
		cache = disk
	}
	counted := &countingStore{StoreOps: f.packs}
	f.p.cache = cache
	recorded := &readRecorder{StoreOps: f.local, reads: &f.p.reads}
	f.reads, f.copies = f.p.readChains(
		recorded,
		f.local,
		func() block.StoreOps { return counted },
		func() block.StoreOps { return nil },
		f.sql,
	)
	waitCache := func() {
		t.Helper()
		if disk == nil {
			return
		}
		ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
		defer cancel()
		if err := disk.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Upload two blocks to the pack store, tracked and untracked, and evict them
	// from the disk.
	tracked, untracked := []byte("tracked pack block"), []byte("untracked pack block")
	trackedRef, _, err := f.wb.PutBlock(f.ctx, tracked, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	untrackedRef, _, err := f.wb.PutBlock(f.ctx, untracked, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.tracked.track(trackedRef)
	f.upload()
	if result := f.evict(); result.Evicted != 2 {
		t.Fatalf("evict = %+v, want both blocks evicted", result)
	}

	// A pack hit fills the cache, not the disk, and the next read skips the
	// pack store.
	before := counted.reads.Load()
	f.read(trackedRef, tracked)
	f.read(untrackedRef, untracked)
	waitCache()
	if n := counted.reads.Load() - before; n != 2 {
		t.Fatalf("first reads touched the pack store %d times, want 2", n)
	}
	if f.held(trackedRef) || f.held(untrackedRef) || f.status().Pending != 0 {
		t.Fatal("a pack hit filled the disk")
	}
	before = counted.reads.Load()
	f.read(trackedRef, tracked)
	f.read(untrackedRef, untracked)
	if n := counted.reads.Load() - before; n != 0 {
		t.Fatalf("cached reads touched the pack store %d times", n)
	}

	// A block only the fallback holds fills the disk with its marker, and the
	// cache stays out of it.
	sqlData := []byte("sql block")
	sqlRef := f.putSQL(sqlData)
	f.read(sqlRef, sqlData)
	f.waitHeld(sqlRef)
	waitCache()
	cacheHolds := func(ref *block.BlockRef) bool {
		t.Helper()
		_, found, err := cache.Reads(func() block.StoreOps { return newLeafStore() })().GetBlock(f.ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	if !cacheHolds(trackedRef) || !cacheHolds(untrackedRef) || cacheHolds(sqlRef) {
		t.Fatal("the cache does not hold exactly the pack hits")
	}

	// A closed disk cache reads as a miss, so the read goes to the pack store.
	if disk == nil {
		return
	}
	if err := disk.Close(); err != nil {
		t.Fatal(err)
	}
	before = counted.reads.Load()
	f.read(trackedRef, tracked)
	if n := counted.reads.Load() - before; n != 1 {
		t.Fatalf("read with a failed cache touched the pack store %d times, want 1", n)
	}
}
