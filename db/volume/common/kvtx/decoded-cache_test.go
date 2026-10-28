package kvtx

import (
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// decodedCacheReader reads blocks of v through one shared decoded cache.
func decodedCacheReader(t *testing.T, v *Volume) (*block.DecodedBlockCache, func(*block.BlockRef) (block.Block, error)) {
	// Share one decoded cache between transactions on the Volume.
	t.Helper()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { decodedBlocks.Close() })
	return decodedBlocks, func(ref *block.BlockRef) (block.Block, error) {
		tx, cursor := block.NewTransaction(v, nil, ref, nil)
		tx.SetDecodedBlockCache(decodedBlocks)
		return cursor.Unmarshal(t.Context(), block_mock.NewExampleBlock)
	}
}

// readTwiceFromPool reads ref twice and checks that the second read hit the
// shared pool.
func readTwiceFromPool(t *testing.T, decodedBlocks *block.DecodedBlockCache, read func(*block.BlockRef) (block.Block, error), ref *block.BlockRef) {
	// Read once to fill the pool.
	t.Helper()
	if _, err := read(ref); err != nil {
		t.Fatal(err)
	}

	// Read again and count the hit.
	hits := decodedBlocks.Snapshot().Hits
	if _, err := read(ref); err != nil {
		t.Fatal(err)
	}
	if got := decodedBlocks.Snapshot().Hits; got != hits+1 {
		t.Fatalf("second read of a Volume block missed the pool: hits %d -> %d", hits, got)
	}
}

// assertSweptUnreadable checks that the Volume no longer holds ref and that a
// transaction sharing the cache gets ErrNotFound, not a decoded copy of a
// block no store holds.
func assertSweptUnreadable(t *testing.T, v *Volume, read func(*block.BlockRef) (block.Block, error), ref *block.BlockRef) {
	// The store no longer holds the block.
	t.Helper()
	if found, err := v.GetBlockExists(t.Context(), ref); err != nil || found {
		t.Fatalf("Volume holds the swept block: found=%v err=%v", found, err)
	}

	// The cache does not serve it.
	blk, err := read(ref)
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("transaction read a block the sweep removed: blk=%v err=%v", blk, err)
	}
}

// TestVolumeSweepInvalidatesDecodedBlocks checks that the unreferenced sweep
// reports its removals to the decoded-block cache.
func TestVolumeSweepInvalidatesDecodedBlocks(t *testing.T) {
	// Store a block and read it twice through the shared pool.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	decodedBlocks, read := decodedCacheReader(t, v)
	ref, _, err := block.PutBlock(ctx, v, &block_mock.Example{Msg: "swept"})
	if err != nil {
		t.Fatal(err)
	}
	readTwiceFromPool(t, decodedBlocks, read, ref)

	// Mark the block unreferenced and sweep it.
	rg := v.GetRefGraph()
	node := block_gc.BlockIRI(ref)
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{{Subject: block_gc.NodeUnreferenced, Object: node}}, nil); err != nil {
		t.Fatal(err)
	}
	removed, err := v.SweepUnreferenced(ctx, rg, []string{node})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(removed, node) {
		t.Fatalf("sweep removed %v, want %s", removed, node)
	}
	assertSweptUnreadable(t, v, read, ref)
}

// TestVolumeBlockPassInvalidatesDecodedBlocks checks that a block pass of the
// collector reports the blocks it sweeps to the decoded-block cache.
func TestVolumeBlockPassInvalidatesDecodedBlocks(t *testing.T) {
	// Open a Volume read through a shared decoded cache.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	decodedBlocks, read := decodedCacheReader(t, v)

	// Publish a block owned by a bucket and read it twice through the pool.
	p := publicationFor(t, "pass", "", "one")
	data, err := (&block_mock.Example{Msg: "block pass"}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Entries[0].Ref, p.Entries[0].Data = ref, data
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}
	readTwiceFromPool(t, decodedBlocks, read, ref)

	// Release it and run a block pass, which sweeps it.
	orphanPublication(t, v, p)
	stats, err := block_gc.NewCollector(v.refGraph.(*transactionRefGraph), v, nil).Collect(ctx)
	if err != nil || stats.BlocksSwept != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	assertSweptUnreadable(t, v, read, ref)
}
