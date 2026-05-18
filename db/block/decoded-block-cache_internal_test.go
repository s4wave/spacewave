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

	if err := decodedBlocks.Store(ctx, nil, decodedBlockCacheKey{
		ref:       refKey,
		blockType: "db/block.decodedBlockCacheIndexTestBlock",
		transform: DecodedBlockCacheNoTransformKey,
		trust:     decodedBlockCacheTrustKey,
	}, ref, &blk, data); err != nil {
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
