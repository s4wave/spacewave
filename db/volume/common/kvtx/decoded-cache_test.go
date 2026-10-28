package kvtx

import (
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestVolumeSweepInvalidatesDecodedBlocks checks that the collector's sweep
// reports its removals to the decoded-block cache. A block read from the
// Volume is shared with the next transaction; after SweepUnreferenced
// removes it, a transaction sharing the same cache must get ErrNotFound, not
// a decoded copy of a block no store holds.
func TestVolumeSweepInvalidatesDecodedBlocks(t *testing.T) {
	// Share one decoded cache between transactions on the Volume.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer decodedBlocks.Close()
	read := func(ref *block.BlockRef) (block.Block, error) {
		tx, cursor := block.NewTransaction(v, nil, ref, nil)
		tx.SetDecodedBlockCache(decodedBlocks)
		return cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	}

	// Store a block, read it, and read it again from the shared pool.
	ref, _, err := block.PutBlock(ctx, v, &block_mock.Example{Msg: "swept"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read(ref); err != nil {
		t.Fatal(err)
	}
	hits := decodedBlocks.Snapshot().Hits
	if _, err := read(ref); err != nil {
		t.Fatal(err)
	}
	if got := decodedBlocks.Snapshot().Hits; got != hits+1 {
		t.Fatalf("second read of a Volume block missed the pool: hits %d -> %d", hits, got)
	}

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
	if found, err := v.GetBlockExists(ctx, ref); err != nil || found {
		t.Fatalf("Volume holds the swept block: found=%v err=%v", found, err)
	}

	// A transaction sharing the cache must not find the swept block.
	blk, err := read(ref)
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("transaction read a block the sweep removed: blk=%v err=%v", blk, err)
	}
}
