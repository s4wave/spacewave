package block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestDecodedBlockCacheSkipsStagedBlocks reproduces the decoded cache
// handing one transaction's staged block to another. A transaction reads back
// a block it staged but never published, then discards its staged writes. A
// second transaction sharing the lifecycle's decoded cache must not decode
// that block: no store holds it, and a commit that read it would link a ref
// to a missing block.
func TestDecodedBlockCacheSkipsStagedBlocks(t *testing.T) {
	// Share one decoded cache between transactions on one store, as an
	// object lifecycle does.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Stage a block in the first transaction and read it back through the
	// transaction's staged store.
	staging, cursor := block.NewTransaction(store, nil, nil, nil)
	staging.SetDecodedBlockCache(decodedBlocks)
	staged := staging.StageWrites(ctx, store)
	ref, _, err := block.PutBlock(ctx, staged, &block_mock.Example{Msg: "staged, never published"})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRefAtCursor(ref, true)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatalf("staging transaction read its own block: %v", err)
	}

	// Discard the staged writes: the block never reaches the store.
	staging.DiscardStagedWrites()
	if found, err := store.GetBlockExists(ctx, ref); err != nil || found {
		t.Fatalf("store holds the discarded block: found=%v err=%v", found, err)
	}

	// A second transaction on the same store and cache must not find it.
	reader, readCursor := block.NewTransaction(store, nil, ref, nil)
	reader.SetDecodedBlockCache(decodedBlocks)
	blk, err := readCursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("second transaction read a block no store holds: blk=%v err=%v", blk, err)
	}
}
