package block_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

func TestReadCounterRecordsUnmarshalMissBaseline(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)

	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "counter"})
	if err != nil {
		t.Fatal(err.Error())
	}
	_, cursor := block.NewTransaction(store, nil, ref, nil)

	opCtx, counter := block.WithReadCounter(ctx)
	blk, err := cursor.Unmarshal(opCtx, func() block.Block { return &block_mock.Example{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	if blk.(*block_mock.Example).GetMsg() != "counter" {
		t.Fatalf("decoded message = %q", blk.(*block_mock.Example).GetMsg())
	}

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 || snapshot.BlockReadBytes == 0 || snapshot.BlockReadMissCount != 0 {
		t.Fatalf("unexpected block read counters: %+v", snapshot)
	}
	if snapshot.DecodedBlockUnmarshalCount != 1 || snapshot.DecodedBlockUnmarshalBytes == 0 {
		t.Fatalf("unexpected unmarshal counters: %+v", snapshot)
	}
	if snapshot.DecodedBlockCacheAttemptCount != 1 ||
		snapshot.DecodedBlockCacheMissCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockCloneCount != 0 {
		t.Fatalf("unexpected decoded cache baseline counters: %+v", snapshot)
	}
}

func TestReadOperationDecodedBlockCacheClonesHits(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)

	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "cached"})
	if err != nil {
		t.Fatal(err.Error())
	}

	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithReadOperationStore(opCtx, store)

	_, firstCursor := block.NewTransaction(store, nil, ref, nil)
	first, err := firstCursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstExample := first.(*block_mock.Example)
	firstExample.Msg = "mutated"

	_, secondCursor := block.NewTransaction(store, nil, ref, nil)
	second, err := secondCursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	secondExample := second.(*block_mock.Example)
	if secondExample.GetMsg() != "cached" {
		t.Fatalf("cached clone msg = %q, want cached", secondExample.GetMsg())
	}
	secondExample.Msg = "mutated again"

	_, thirdCursor := block.NewTransaction(store, nil, ref, nil)
	third, err := thirdCursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := third.(*block_mock.Example).GetMsg(); got != "cached" {
		t.Fatalf("second cached clone msg = %q, want cached", got)
	}

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 || snapshot.DecodedBlockUnmarshalCount != 1 {
		t.Fatalf("unexpected storage/unmarshal counters: %+v", snapshot)
	}
	if snapshot.DecodedBlockCacheAttemptCount != 3 ||
		snapshot.DecodedBlockCacheMissCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 2 ||
		snapshot.DecodedBlockCloneCount != 2 ||
		snapshot.DecodedBlockUncloneableCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected decoded cache counters: %+v", snapshot)
	}
}

func TestReadOperationDecodedBlockCacheReturnsCloneHits(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)

	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "cached"})
	if err != nil {
		t.Fatal(err.Error())
	}
	_, first := block.NewTransaction(store, nil, ref, nil)
	_, second := block.NewTransaction(store, nil, ref, nil)
	ctor := func() block.Block { return &block_mock.Example{} }

	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithReadOperationStore(opCtx, store)
	firstBlock, err := first.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstExample := firstBlock.(*block_mock.Example)
	firstExample.Msg = "mutated"

	secondBlock, err := second.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}
	secondExample := secondBlock.(*block_mock.Example)
	if secondExample.GetMsg() != "cached" {
		t.Fatalf("cached clone message = %q, want cached", secondExample.GetMsg())
	}
	if firstExample == secondExample {
		t.Fatal("cache hit returned the first decoded block instance")
	}

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 ||
		snapshot.DecodedBlockCloneCount != 1 ||
		snapshot.DecodedBlockUncloneableCount != 0 {
		t.Fatalf("unexpected decoded cache counters: %+v", snapshot)
	}
}
