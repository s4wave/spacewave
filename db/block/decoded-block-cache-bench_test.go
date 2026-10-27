package block

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"
)

// BenchmarkDecodedBlockCacheLookupParallel measures hits from many goroutines
// on a pool holding 10,000 entries.
func BenchmarkDecodedBlockCacheLookupParallel(b *testing.B) {
	// Fill the pool, then look up its entries from every goroutine.
	ctx := context.Background()
	cache, keys := newDecodedBlockCacheBench(b, 10000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := rand.IntN(len(keys))
		for pb.Next() {
			_, _, _ = cache.Lookup(ctx, nil, keys[i], true)
			i = (i + 1) % len(keys)
		}
	})
}

// BenchmarkDecodedBlockCacheCommitParallel measures many goroutines that each
// store a new entry every sixteen lookups, as readers of new heads do.
func BenchmarkDecodedBlockCacheCommitParallel(b *testing.B) {
	// Fill the pool, then mix lookups with stores of new entries.
	ctx := context.Background()
	cache, keys := newDecodedBlockCacheBench(b, 10000)
	_, _, blk, data := newDecodedBlockCacheTestEntry(b, "fresh")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := rand.IntN(len(keys))
		for pb.Next() {
			if i%16 == 0 {
				key := keys[i]
				key.ref = "fresh " + strconv.Itoa(rand.Int())
				_ = cache.Store(ctx, nil, cache.storeToken(key.ref), key, blk, len(data), true)
			} else {
				_, _, _ = cache.Lookup(ctx, nil, keys[i], true)
			}
			i = (i + 1) % len(keys)
		}
	})
}

// newDecodedBlockCacheBench returns a 256 MiB pool holding n entries read once.
func newDecodedBlockCacheBench(b *testing.B, n int) (*DecodedBlockCache, []decodedBlockCacheKey) {
	// Open the pool.
	cache, err := NewDecodedBlockCacheWithOptions(DecodedBlockCacheOptions{MaxCost: 256 << 20})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(cache.Close)

	// Store each entry and read it once, so it can leave probation.
	keys := make([]decodedBlockCacheKey, n)
	for i := range keys {
		_, key, blk, data := newDecodedBlockCacheTestEntry(b, "entry "+strconv.Itoa(i))
		storeDecodedBlockCacheTestEntry(b, cache, key, blk, data)
		_, _, _ = cache.Lookup(context.Background(), nil, key, true)
		keys[i] = key
	}
	return cache, keys
}
