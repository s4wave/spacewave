package block

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestDecodedBlockCacheNewHotPage reproduces a World commit's effect on a full
// cache: the pages of older heads were read often, and the commit's new page
// is read by many readers at once, each of which stores it after a miss. The
// first store must serve every later reader, however hot the older pages were.
func TestDecodedBlockCacheNewHotPage(t *testing.T) {
	for _, oldPages := range []int{0, 1000} {
		t.Run(fmt.Sprintf("old=%d", oldPages), func(t *testing.T) { testNewHotPage(t, oldPages) })
	}
}

// testNewHotPage runs the new-page rounds on a pool first filled with
// oldPages pages read many times.
func testNewHotPage(t *testing.T, oldPages int) {
	// Open a small pool and fill it with older pages read many times.
	ctx := context.Background()
	cache, err := NewDecodedBlockCacheWithOptions(DecodedBlockCacheOptions{MaxCost: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	const oldReads = 20
	for i := range oldPages {
		_, key, blk, data := newDecodedBlockCacheTestEntry(t, fmt.Sprintf("old %d %s", i, strings.Repeat("x", 1024)))
		storeDecodedBlockCacheTestEntry(t, cache, key, blk, data)
		for range oldReads {
			_, _, _ = cache.Lookup(ctx, nil, key, true)
		}
	}

	// Publish new pages one at a time, each read by many readers that store it
	// on a miss, as Cursor.Unmarshal does.
	const rounds, readers, attempts = 50, 64, 200
	var totalStores, uncached int
	for round := range rounds {
		_, key, blk, data := newDecodedBlockCacheTestEntry(t, fmt.Sprintf("new %d %s", round, strings.Repeat("y", 1024)))
		var stores, missed atomic.Int64
		var wg sync.WaitGroup
		for range readers {
			wg.Go(func() {
				for range attempts {
					if _, ok, _ := cache.Lookup(ctx, nil, key, true); ok {
						return
					}
					stores.Add(1)
					token := cache.storeToken(key.ref)
					_ = cache.Store(ctx, nil, token, key, blk, len(data), DurableReadGeneration(), true)
					runtime.Gosched()
				}
				missed.Add(1)
			})
		}
		wg.Wait()
		totalStores += int(stores.Load())
		if missed.Load() != 0 {
			uncached++
		}
	}

	// Every new page was cached, after about one store each. Only readers that
	// missed before the first store landed may store again.
	snap := cache.Snapshot()
	perPage := float64(totalStores) / rounds
	t.Logf("%d new pages, %d readers each: %.1f stores per page; pool stores %d rejections %d",
		rounds, readers, perPage, snap.Stores, snap.Rejections)
	if uncached != 0 || snap.Rejections != 0 {
		t.Fatalf("%d new pages never cached, %d stores rejected", uncached, snap.Rejections)
	}
	if perPage > readers/4 {
		t.Fatalf("%.1f stores per new page, want at most %d", perPage, readers/4)
	}
}
