package block

import (
	"context"
	"testing"
)

func TestDecodedBlockCacheInvalidatedStoreTokenSkipsStore(t *testing.T) {
	// Create a decoded cache and candidate entry for invalidation.
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "removed before admission")

	// Invalidate the candidate token before submitting its block.
	token := decodedBlocks.storeToken(key.ref)
	decodedBlocks.InvalidateRef(ctx, ref)
	if err := decodedBlocks.Store(ctx, nil, token, key, ref, blk, data, false); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

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
	storeDecodedBlockCacheTestEntry(t, decodedBlocks, ref, key, blk, data)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || !ok {
		t.Fatalf("stored lookup ok=%v err=%v, want hit", ok, err)
	}
	decodedBlocks.InvalidateRef(ctx, ref)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("lookup after InvalidateRef ok=%v err=%v, want miss", ok, err)
	}

	// Restore the entry and verify whole-cache invalidation removes its hit.
	storeDecodedBlockCacheTestEntry(t, decodedBlocks, ref, key, blk, data)
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
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "scoped entry")

	// Verify cache scopes share their pool but keep entries isolated.
	storeDecodedBlockCacheTestEntry(t, first, ref, key, blk, data)
	if _, ok, err := first.Lookup(ctx, nil, key, false); err != nil || !ok {
		t.Fatalf("owning scope lookup ok=%v err=%v, want hit", ok, err)
	}
	if _, ok, err := second.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("other scope lookup ok=%v err=%v, want miss", ok, err)
	}
	if first.pool != second.pool {
		t.Fatal("NewDecodedBlockCache scopes use different pools")
	}

	// Close the first scope and verify the second scope retains its pool.
	first.Close()
	if _, ok, err := first.Lookup(ctx, nil, key, false); err != nil || ok {
		t.Fatalf("closed scope lookup ok=%v err=%v, want miss", ok, err)
	}
	if second.pool.cache == nil {
		t.Fatal("closing a scope closed the shared pool")
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
	t *testing.T,
	contents string,
) (*BlockRef, decodedBlockCacheKey, *decodedBlockCacheTestBlock, []byte) {
	// Create the decoded-cache test block, serialized bytes, and reference.
	t.Helper()
	blk := &decodedBlockCacheTestBlock{data: []byte(contents)}
	data, err := blk.MarshalBlock()
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

// storeDecodedBlockCacheTestEntry stores blk with a current token and waits
// for the pool to admit it.
func storeDecodedBlockCacheTestEntry(
	t *testing.T,
	decodedBlocks *DecodedBlockCache,
	ref *BlockRef,
	key decodedBlockCacheKey,
	blk *decodedBlockCacheTestBlock,
	data []byte,
) {
	t.Helper()
	token := decodedBlocks.storeToken(key.ref)
	if err := decodedBlocks.Store(context.Background(), nil, token, key, ref, blk, data, false); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()
}

type decodedBlockCacheTestBlock struct {
	data []byte
}

func (b *decodedBlockCacheTestBlock) MarshalBlock() ([]byte, error) {
	return append([]byte(nil), b.data...), nil
}

func (b *decodedBlockCacheTestBlock) UnmarshalBlock(data []byte) error {
	b.data = append(b.data[:0], data...)
	return nil
}

func (b *decodedBlockCacheTestBlock) CloneBlock() (Block, error) {
	return &decodedBlockCacheTestBlock{data: append([]byte(nil), b.data...)}, nil
}

func (b *decodedBlockCacheTestBlock) SizeVT() int {
	return len(b.data)
}
