package block

import (
	"context"
	"strings"
	"testing"
)

func TestDecodedBlockCacheInvalidatedStoreTokenSkipsStore(t *testing.T) {
	// Create a decoded cache and candidate entry for invalidation.
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "removed before store")

	// Invalidate the candidate token before submitting its block.
	token := decodedBlocks.storeToken(key.ref)
	decodedBlocks.InvalidateRef(ctx, ref)
	if err := decodedBlocks.Store(ctx, nil, token, key, blk, len(data), DurableReadGeneration(), false); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the invalidated token cannot populate the cache.
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("invalidated store token lookup ok=%v err=%v, want miss", ok, err)
	}
}

func TestDecodedBlockCacheInvalidationMakesEntriesStale(t *testing.T) {
	// Create a decoded cache and reusable entry for invalidation checks.
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "cached entry")

	// Store the entry and verify reference invalidation removes its hit.
	storeDecodedBlockCacheTestEntry(t, decodedBlocks, key, blk, data)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || !ok {
		t.Fatalf("stored lookup ok=%v err=%v, want hit", ok, err)
	}
	decodedBlocks.InvalidateRef(ctx, ref)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("lookup after InvalidateRef ok=%v err=%v, want miss", ok, err)
	}

	// Restore the entry and verify whole-cache invalidation removes its hit.
	storeDecodedBlockCacheTestEntry(t, decodedBlocks, key, blk, data)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || !ok {
		t.Fatalf("restored lookup ok=%v err=%v, want hit", ok, err)
	}
	decodedBlocks.InvalidateAll(ctx)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("lookup after InvalidateAll ok=%v err=%v, want miss", ok, err)
	}
}

func TestDecodedBlockCacheScopesShareBudgetNotEntries(t *testing.T) {
	// Create independent decoded-cache scopes and a shared test entry.
	ctx := context.Background()
	first, second := NewDecodedBlockCache(), NewDecodedBlockCache()
	defer first.Close()
	defer second.Close()
	_, key, blk, data := newDecodedBlockCacheTestEntry(t, "scoped entry")

	// Verify cache scopes share their pool but keep entries isolated.
	storeDecodedBlockCacheTestEntry(t, first, key, blk, data)
	if _, ok, err := first.Lookup(ctx, nil, key, false); err != nil || !ok {
		t.Fatalf("owning scope lookup ok=%v err=%v, want hit", ok, err)
	}
	if _, ok, err := second.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("other scope lookup ok=%v err=%v, want miss", ok, err)
	}
	if first.pool != second.pool {
		t.Fatal("NewDecodedBlockCache scopes use different pools")
	}

	// Close the first scope and verify the second scope retains its entries.
	storeDecodedBlockCacheTestEntry(t, second, key, blk, data)
	first.Close()
	if _, ok, err := first.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("closed scope lookup ok=%v err=%v, want miss", ok, err)
	}
	if _, ok, err := second.Lookup(ctx, nil, key, false); err != nil || !ok {
		t.Fatalf("closing a scope cleared the shared pool: ok=%v err=%v", ok, err)
	}
}

// newTestDecodedBlockCache constructs a cache with a private default pool.
func newTestDecodedBlockCache(t *testing.T) *DecodedBlockCache {
	// Create a private decoded cache and register its cleanup.
	t.Helper()
	decodedBlocks, err := NewDecodedBlockCacheWithOptions(DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(decodedBlocks.Close)
	return decodedBlocks
}

// newDecodedBlockCacheTestEntry builds a block holding contents with its ref
// and cache key.
func newDecodedBlockCacheTestEntry(
	t testing.TB,
	contents string,
) (*BlockRef, decodedBlockCacheKey, *decodedBlockCacheTestBlock, []byte) {
	// Create the decoded-cache test block, serialized bytes, and reference.
	t.Helper()
	blk := &decodedBlockCacheTestBlock{data: []byte(contents)}
	data, err := blk.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	ref, err := BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the exact decoded-cache key from the block reference.
	refKey, ok := decodedBlockCacheRefKey(ref)
	if !ok {
		t.Fatal("decoded block cache ref key was empty")
	}
	key := decodedBlockCacheKey{
		ref:       refKey,
		blockType: "db/block.decodedBlockCacheTestBlock",
		transform: DecodedBlockCacheNoTransformKey,
		trust:     decodedBlockCacheTrustKey,
	}
	return ref, key, blk, data
}

// storeDecodedBlockCacheTestEntry stores blk with a current token.
func storeDecodedBlockCacheTestEntry(
	t testing.TB,
	decodedBlocks *DecodedBlockCache,
	key decodedBlockCacheKey,
	blk *decodedBlockCacheTestBlock,
	data []byte,
) {
	t.Helper()
	token := decodedBlocks.storeToken(key.ref)
	if err := decodedBlocks.Store(context.Background(), nil, token, key, blk, len(data), DurableReadGeneration(), false); err != nil {
		t.Fatal(err.Error())
	}
}

type decodedBlockCacheTestBlock struct {
	data []byte
}

func (b *decodedBlockCacheTestBlock) MarshalVT() ([]byte, error) {
	return append([]byte(nil), b.data...), nil
}

func (b *decodedBlockCacheTestBlock) UnmarshalVT(data []byte) error {
	b.data = append(b.data[:0], data...)
	return nil
}

func (b *decodedBlockCacheTestBlock) CloneBlock() (Block, error) {
	return &decodedBlockCacheTestBlock{data: append([]byte(nil), b.data...)}, nil
}

func (b *decodedBlockCacheTestBlock) SizeVT() int {
	return len(b.data)
}

// TestSetSharedDecodedBlockCacheMaxCost resizes the process-wide pool.
func TestSetSharedDecodedBlockCacheMaxCost(t *testing.T) {
	// Raise the budget, restoring it afterward.
	cache := NewDecodedBlockCache()
	defer SetSharedDecodedBlockCacheMaxCost(cache.MaxCost())
	SetSharedDecodedBlockCacheMaxCost(1 << 30)

	// Expect every scope to report the new budget.
	if got := cache.MaxCost(); got != 1<<30 {
		t.Fatalf("max cost %d, want %d", got, 1<<30)
	}
	if got := cache.Snapshot().MaxCost; got != 1<<30 {
		t.Fatalf("snapshot max cost %d, want %d", got, 1<<30)
	}
}

// TestDecodedBlockCacheKeyExactLength checks that a pool key length-prefixes
// each part, including parts whose lengths cross a digit, and that building
// one allocates only the key.
func TestDecodedBlockCacheKeyExactLength(t *testing.T) {
	// Encode parts of 0, 9, 10 and 100 bytes.
	c := &DecodedBlockCache{scope: "s/"}
	key := decodedBlockCacheKey{
		ref:       strings.Repeat("r", 9),
		blockType: strings.Repeat("b", 10),
		transform: strings.Repeat("x", 100),
	}
	want := "s/9:" + key.ref + "10:" + key.blockType + "100:" + key.transform + "0:"
	if got := c.cacheKey(key); got != want {
		t.Fatalf("cache key %q, want %q", got, want)
	}

	// Building the key allocates it once.
	if allocs := testing.AllocsPerRun(100, func() { _ = c.cacheKey(key) }); allocs != 1 {
		t.Fatalf("building a cache key allocates %v times, want 1", allocs)
	}
}

func TestDecodedBlockCacheOffPoolServesNothing(t *testing.T) {
	// Store an entry, then turn the pool off.
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	_, key, blk, data := newDecodedBlockCacheTestEntry(t, "cached before off")
	storeDecodedBlockCacheTestEntry(t, decodedBlocks, key, blk, data)
	decodedBlocks.pool.off.Store(true)

	// Verify the stored entry and a new store both miss.
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("lookup after off ok=%v err=%v, want miss", ok, err)
	}
	storeDecodedBlockCacheTestEntry(t, decodedBlocks, key, blk, data)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("lookup of a store after off ok=%v err=%v, want miss", ok, err)
	}
}
