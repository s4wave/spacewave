package block

import (
	"context"
	"testing"
)

func TestDecodedBlockCacheInvalidatedStoreTokenSkipsStore(t *testing.T) {
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "removed before admission")

	token := decodedBlocks.storeToken(key.ref)
	decodedBlocks.InvalidateRef(ctx, ref)
	if err := decodedBlocks.Store(ctx, nil, token, key, ref, blk, data); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key); err != nil || ok {
		t.Fatalf("invalidated store token lookup ok=%v err=%v, want miss", ok, err)
	}
}

func TestDecodedBlockCacheInvalidationMakesEntriesStale(t *testing.T) {
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "cached entry")

	storeDecodedBlockCacheTestEntry(t, decodedBlocks, ref, key, blk, data)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key); err != nil || !ok {
		t.Fatalf("stored lookup ok=%v err=%v, want hit", ok, err)
	}
	decodedBlocks.InvalidateRef(ctx, ref)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key); err != nil || ok {
		t.Fatalf("lookup after InvalidateRef ok=%v err=%v, want miss", ok, err)
	}

	storeDecodedBlockCacheTestEntry(t, decodedBlocks, ref, key, blk, data)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key); err != nil || !ok {
		t.Fatalf("restored lookup ok=%v err=%v, want hit", ok, err)
	}
	decodedBlocks.InvalidateAll(ctx)
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key); err != nil || ok {
		t.Fatalf("lookup after InvalidateAll ok=%v err=%v, want miss", ok, err)
	}
}

func TestDecodedBlockCacheScopesShareBudgetNotEntries(t *testing.T) {
	ctx := context.Background()
	first, second := NewDecodedBlockCache(), NewDecodedBlockCache()
	defer first.Close()
	defer second.Close()
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "scoped entry")

	storeDecodedBlockCacheTestEntry(t, first, ref, key, blk, data)
	if _, ok, err := first.Lookup(ctx, nil, key); err != nil || !ok {
		t.Fatalf("owning scope lookup ok=%v err=%v, want hit", ok, err)
	}
	if _, ok, err := second.Lookup(ctx, nil, key); err != nil || ok {
		t.Fatalf("other scope lookup ok=%v err=%v, want miss", ok, err)
	}
	if first.pool != second.pool {
		t.Fatal("NewDecodedBlockCache scopes use different pools")
	}

	first.Close()
	if _, ok, err := first.Lookup(ctx, nil, key); err != nil || ok {
		t.Fatalf("closed scope lookup ok=%v err=%v, want miss", ok, err)
	}
	if second.pool.cache == nil {
		t.Fatal("closing a scope closed the shared pool")
	}
}

// newTestDecodedBlockCache constructs a cache with a private default pool.
func newTestDecodedBlockCache(t *testing.T) *DecodedBlockCache {
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
	if err := decodedBlocks.Store(context.Background(), nil, token, key, ref, blk, data); err != nil {
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
