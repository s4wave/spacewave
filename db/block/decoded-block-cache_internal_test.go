package block

import (
	"context"
	"testing"
)

func TestDecodedBlockCacheRejectedStorePrunesRefIndex(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := NewDecodedBlockCacheWithOptions(DecodedBlockCacheOptions{
		MaxCost: 1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	blk := decodedBlockCacheIndexTestBlock{data: []byte("entry larger than the cache budget")}
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
		blockType: "db/block.decodedBlockCacheIndexTestBlock",
		transform: DecodedBlockCacheNoTransformKey,
		trust:     decodedBlockCacheTrustKey,
	}
	if err := decodedBlocks.Store(ctx, nil, decodedBlocks.storeToken(refKey), key, ref, &blk, data); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	decodedBlocks.mtx.Lock()
	refEntries := len(decodedBlocks.byRef)
	hashEntries := len(decodedBlocks.byHash)
	decodedBlocks.mtx.Unlock()
	if refEntries != 0 || hashEntries != 0 {
		t.Fatalf("rejected cache entry left index refs: byRef=%d byHash=%d", refEntries, hashEntries)
	}
}

func TestDecodedBlockCacheInvalidatedStoreTokenSkipsStore(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := NewDecodedBlockCacheWithOptions(DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	blk := decodedBlockCacheIndexTestBlock{data: []byte("removed before admission")}
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
		blockType: "db/block.decodedBlockCacheIndexTestBlock",
		transform: DecodedBlockCacheNoTransformKey,
		trust:     decodedBlockCacheTrustKey,
	}
	token := decodedBlocks.storeToken(refKey)
	decodedBlocks.InvalidateRef(ctx, ref)

	if err := decodedBlocks.Store(ctx, nil, token, key, ref, &blk, data); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()
	if _, ok, err := decodedBlocks.Lookup(ctx, nil, key); err != nil || ok {
		t.Fatalf("invalidated store token lookup ok=%v err=%v, want miss", ok, err)
	}
}

func TestDecodedBlockCacheOldCallbackDoesNotPruneNewIndexEntry(t *testing.T) {
	decodedBlocks, err := NewDecodedBlockCacheWithOptions(DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	refKey := "ref-a"
	key := decodedBlockCacheKey{
		ref:       refKey,
		blockType: "db/block.decodedBlockCacheIndexTestBlock",
		transform: DecodedBlockCacheNoTransformKey,
		trust:     decodedBlockCacheTrustKey,
	}.String()
	h := decodedBlockCacheHashFor(key)

	decodedBlocks.mtx.Lock()
	oldGeneration := decodedBlocks.recordRefKeyLocked(refKey, key)
	decodedBlocks.mtx.Unlock()
	decodedBlocks.takeRefKeys(refKey)

	decodedBlocks.mtx.Lock()
	newGeneration := decodedBlocks.recordRefKeyLocked(refKey, key)
	decodedBlocks.removeRefKeyHashGenerationLocked(h, oldGeneration)
	_, stillTracked := decodedBlocks.byRef[refKey][key]
	decodedBlocks.removeRefKeyHashGenerationLocked(h, newGeneration)
	_, removed := decodedBlocks.byRef[refKey]
	decodedBlocks.mtx.Unlock()

	if !stillTracked {
		t.Fatal("old async callback pruned newer decoded-cache index entry")
	}
	if removed {
		t.Fatal("current async callback did not prune decoded-cache index entry")
	}
}

func TestDecodedBlockCacheRejectedDuplicateKeepsResidentIndexEntry(t *testing.T) {
	decodedBlocks, err := NewDecodedBlockCacheWithOptions(DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	refKey := "ref-a"
	key := decodedBlockCacheKey{
		ref:       refKey,
		blockType: "db/block.decodedBlockCacheIndexTestBlock",
		transform: DecodedBlockCacheNoTransformKey,
		trust:     decodedBlockCacheTrustKey,
	}.String()
	h := decodedBlockCacheHashFor(key)

	decodedBlocks.mtx.Lock()
	residentGeneration := decodedBlocks.recordRefKeyLocked(refKey, key)
	rejectedGeneration := decodedBlocks.recordRefKeyLocked(refKey, key)
	decodedBlocks.removeRefKeyHashGenerationLocked(h, rejectedGeneration)
	_, stillTracked := decodedBlocks.byRef[refKey][key]
	decodedBlocks.removeRefKeyHashGenerationLocked(h, residentGeneration)
	_, removed := decodedBlocks.byRef[refKey]
	decodedBlocks.mtx.Unlock()

	if !stillTracked {
		t.Fatal("rejected duplicate admission pruned resident decoded-cache index entry")
	}
	if removed {
		t.Fatal("resident generation did not prune decoded-cache index entry")
	}
}

type decodedBlockCacheIndexTestBlock struct {
	data []byte
}

func (b *decodedBlockCacheIndexTestBlock) MarshalBlock() ([]byte, error) {
	return append([]byte(nil), b.data...), nil
}

func (b *decodedBlockCacheIndexTestBlock) UnmarshalBlock(data []byte) error {
	b.data = append(b.data[:0], data...)
	return nil
}

func (b *decodedBlockCacheIndexTestBlock) SizeVT() int {
	return len(b.data)
}
