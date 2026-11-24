package block

import (
	"context"
	"strconv"
	"testing"
)

func TestDecodedBlockCacheResidentYieldsLiveEntries(t *testing.T) {
	// Store three entries in one scope and one in another scope of the pool.
	ctx := context.Background()
	decodedBlocks := newTestDecodedBlockCache(t)
	other := newDecodedBlockCacheScope(decodedBlocks.pool, false)
	defer other.Close()
	keys := make([]decodedBlockCacheKey, 3)
	refs := make([]*BlockRef, 3)
	for i := range keys {
		ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "resident "+strconv.Itoa(i))
		storeDecodedBlockCacheTestEntry(t, decodedBlocks, key, blk, data)
		refs[i], keys[i] = ref, key
	}
	_, otherKey, otherBlk, otherData := newDecodedBlockCacheTestEntry(t, "other scope")
	storeDecodedBlockCacheTestEntry(t, other, otherKey, otherBlk, otherData)

	// Outdate one entry by invalidation and one by a durable removal.
	decodedBlocks.InvalidateRef(ctx, refs[0])
	InvalidateRemovedBlock(refs[1])

	// Walk the scope: only the entry a Lookup still hits remains.
	var got []DecodedBlockResident
	for r := range decodedBlocks.Resident() {
		got = append(got, r)
	}
	if len(got) != 1 || got[0].RefKey != keys[2].ref || got[0].TypeKey != keys[2].blockType || got[0].Cost <= 0 {
		t.Fatalf("resident = %+v, want only the third entry with a cost", got)
	}
	decoded := &BlockRef{}
	if err := decoded.UnmarshalVT([]byte(got[0].RefKey)); err != nil || !decoded.EqualsRef(refs[2]) {
		t.Fatalf("resident ref key decodes to %v (err %v), want %v", decoded, err, refs[2])
	}
	for i, key := range keys {
		_, hit, err := decodedBlocks.Lookup(ctx, nil, key, false)
		if err != nil || hit != (i == 2) {
			t.Fatalf("lookup %d hit=%v err=%v, want hit only for the yielded entry", i, hit, err)
		}
	}

	// Count by type agrees with the walk, and a closed scope yields nothing.
	counts := decodedBlocks.CountByType()
	if len(counts) != 1 || counts[0].Entries != 1 || counts[0].Cost != got[0].Cost {
		t.Fatalf("CountByType = %+v, want one type with the walked entry", counts)
	}
	decodedBlocks.Close()
	for r := range decodedBlocks.Resident() {
		t.Fatalf("closed scope yielded %+v", r)
	}
}

func TestDecodedBlockCacheResidentAllocsFlat(t *testing.T) {
	// Measure the walk's allocations per entry at n and 10n entries.
	perEntry := func(n int) float64 {
		decodedBlocks := newTestDecodedBlockCache(t)
		for i := range n {
			_, key, blk, data := newDecodedBlockCacheTestEntry(t, "flat "+strconv.Itoa(i))
			storeDecodedBlockCacheTestEntry(t, decodedBlocks, key, blk, data)
		}
		allocs := testing.AllocsPerRun(5, func() {
			for range decodedBlocks.Resident() {
			}
		})
		return allocs / float64(n)
	}

	// The walk's buffer grows once per pass, so allocations per entry fall
	// rather than rise with the entry count.
	small, large := perEntry(500), perEntry(5000)
	if large > small || large > 0.1 {
		t.Fatalf("walk allocations per entry %.3f at 500 and %.3f at 5000, want flat and under 0.1", small, large)
	}
}
