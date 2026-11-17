package block

import (
	"context"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// BenchmarkDecodedPoolAdmitParallel measures fixed goroutine counts that each
// look up a random block of a working set four times the pool and store it on
// a miss, as reads admitted to the pool do once most of them miss. The pool's
// shards evict on most stores. Each count runs with and without a shared read
// budget, which reclaims on every store when the pool joined it.
func BenchmarkDecodedPoolAdmitParallel(b *testing.B) {
	entries := newDecodedPoolAdmitEntries(b, 64<<10)
	for _, shared := range []bool{false, true} {
		for _, goroutines := range []int{16, 64} {
			name := "goroutines=" + strconv.Itoa(goroutines) + "/budget=" + strconv.FormatBool(shared)
			b.Run(name, func(b *testing.B) {
				benchDecodedPoolAdmit(b, entries, goroutines, shared)
			})
		}
	}
}

// decodedPoolAdmitEntry is one block of the benchmark's working set.
type decodedPoolAdmitEntry struct {
	key  decodedBlockCacheKey
	blk  *decodedBlockCacheTestBlock
	size int
}

// newDecodedPoolAdmitEntries builds n blocks of about 1 KiB each.
func newDecodedPoolAdmitEntries(b *testing.B, n int) []decodedPoolAdmitEntry {
	pad := strings.Repeat("x", 1<<10)
	entries := make([]decodedPoolAdmitEntry, n)
	for i := range entries {
		_, key, blk, data := newDecodedBlockCacheTestEntry(b, strconv.Itoa(i)+pad)
		entries[i] = decodedPoolAdmitEntry{key: key, blk: blk, size: len(data)}
	}
	return entries
}

// benchDecodedPoolAdmit runs b.N lookups, split over the goroutines, on a 32
// MiB pool of eight shards, storing each miss.
func benchDecodedPoolAdmit(b *testing.B, entries []decodedPoolAdmitEntry, goroutines int, shared bool) {
	// Open the pool, joined to a budget of its own size when shared.
	pool, err := newDecodedBlockPool(DecodedBlockCacheOptions{MaxCost: 32 << 20})
	if err != nil {
		b.Fatal(err)
	}
	if shared {
		pool.budget = NewReadCacheBudget(32 << 20)
		pool.budget.Join(ReadCacheDecoded, pool)
	}
	cache := newDecodedBlockCacheScope(pool, true)
	b.Cleanup(cache.Close)

	// Warm the pool with one pass over the working set.
	ctx := context.Background()
	for _, e := range entries {
		_ = cache.Store(ctx, nil, cache.storeToken(e.key.ref), e.key, e.blk, e.size, DurableReadGeneration(), true)
	}

	// Look up and admit from every goroutine.
	before := cache.Snapshot()
	b.ReportAllocs()
	b.ResetTimer()
	var wg sync.WaitGroup
	for g := range goroutines {
		n := b.N / goroutines
		if g < b.N%goroutines {
			n++
		}
		wg.Go(func() {
			for range n {
				e := &entries[rand.IntN(len(entries))]
				if _, ok, _ := cache.Lookup(ctx, nil, e.key, true); ok {
					continue
				}
				_ = cache.Store(ctx, nil, cache.storeToken(e.key.ref), e.key, e.blk, e.size, DurableReadGeneration(), true)
			}
		})
	}
	wg.Wait()
	b.StopTimer()

	// Report the share of lookups that missed and stored, and the evictions.
	after := cache.Snapshot()
	b.ReportMetric(float64(after.Stores-before.Stores)/float64(b.N), "stores/op")
	b.ReportMetric(float64(after.Evictions-before.Evictions)/float64(b.N), "evictions/op")
}
