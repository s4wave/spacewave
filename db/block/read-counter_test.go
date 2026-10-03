package block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

func TestReadCounterRecordsUnmarshalMissBaseline(t *testing.T) {
	// Create the backing store for an uncached block read.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)

	// Store a block and open its read cursor.
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "counter"})
	if err != nil {
		t.Fatal(err.Error())
	}
	_, cursor := block.NewTransaction(store, nil, ref, nil)

	// Decode the block through an operation read counter.
	opCtx, counter := block.WithReadCounter(ctx)
	blk, err := cursor.Unmarshal(opCtx, func() block.Block { return &block_mock.Example{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	if blk.(*block_mock.Example).GetMsg() != "counter" {
		t.Fatalf("decoded message = %q", blk.(*block_mock.Example).GetMsg())
	}

	// Verify the uncached read and unmarshal counter totals.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 || snapshot.BlockReadBytes == 0 || snapshot.BlockReadMissCount != 0 {
		t.Fatalf("unexpected block read counters: %+v", snapshot)
	}
	if snapshot.DecodedBlockUnmarshalCount != 1 || snapshot.DecodedBlockUnmarshalBytes == 0 {
		t.Fatalf("unexpected unmarshal counters: %+v", snapshot)
	}
	if snapshot.DecodedBlockCacheAttemptCount != 0 ||
		snapshot.DecodedBlockCacheMissCount != 0 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockCloneCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 1 {
		t.Fatalf("unexpected decoded cache baseline counters: %+v", snapshot)
	}
}

func TestReadOperationDecodedBlockCacheClonesHits(t *testing.T) {
	// Create the backing store for operation-scoped cache reads.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)

	// Store the block that repeated cursors will decode.
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "cached"})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Attach the operation cache and read counter to the context.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithReadOperationStore(opCtx, store)

	// Decode the first block and mutate its returned instance.
	_, firstCursor := block.NewTransaction(store, nil, ref, nil)
	first, err := firstCursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstExample := first.(*block_mock.Example)
	firstExample.Msg = "mutated"

	// Verify the second cursor receives original content and mutate its clone.
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

	// Verify a third cursor still receives the original cached content.
	_, thirdCursor := block.NewTransaction(store, nil, ref, nil)
	third, err := thirdCursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := third.(*block_mock.Example).GetMsg(); got != "cached" {
		t.Fatalf("second cached clone msg = %q, want cached", got)
	}

	// Verify the operation cache avoided repeated reads and unmarshals.
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
	// Create a backing store for clone-on-hit checks.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)

	// Store a block and open two independent read cursors.
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "cached"})
	if err != nil {
		t.Fatal(err.Error())
	}
	_, first := block.NewTransaction(store, nil, ref, nil)
	_, second := block.NewTransaction(store, nil, ref, nil)
	ctor := func() block.Block { return &block_mock.Example{} }

	// Populate the operation cache and mutate the first returned block.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithReadOperationStore(opCtx, store)
	firstBlock, err := first.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstExample := firstBlock.(*block_mock.Example)
	firstExample.Msg = "mutated"

	// Verify the cache hit returns original content in a distinct instance.
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

	// Verify the second decode counted one cache hit and clone.
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

func TestDecodedBlockCacheRejectsRefVerificationMismatch(t *testing.T) {
	// Store the original block for a reference-verification mismatch.
	ctx := context.Background()
	baseStore := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, baseStore, &block_mock.Example{Msg: "original"})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Encode corrupt bytes that do not match the stored reference.
	poison, err := (&block_mock.Example{Msg: "poison"}).MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	store := &corruptingStore{StoreOps: baseStore, data: poison}

	// Create a shared decoded-block cache for the corrupting store.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Decode corrupt store contents twice through the shared cache.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	for range 2 {
		_, cursor := block.NewTransaction(store, nil, ref, nil)
		blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got := blk.(*block_mock.Example).GetMsg(); got != "poison" {
			t.Fatalf("decoded message = %q, want poison", got)
		}
		decodedBlocks.Wait()
	}

	// Verify reference mismatches bypass shared cache retention.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 2 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 2 {
		t.Fatalf("verification mismatch should bypass shared cache: %+v", snapshot)
	}
}

func TestDecodedBlockCacheBypassesUnknownTransform(t *testing.T) {
	// Store a block and open a shared cache for an unknown transform.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "transform"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Decode the block twice through the unrecognized transform.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	for range 2 {
		_, cursor := block.NewTransaction(store, passthroughTransformer{}, ref, nil)
		blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got := blk.(*block_mock.Example).GetMsg(); got != "transform" {
			t.Fatalf("decoded message = %q, want transform", got)
		}
	}

	// Verify unknown transforms bypass shared cache lookups.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheAttemptCount != 0 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 2 {
		t.Fatalf("unknown transform should bypass shared cache: %+v", snapshot)
	}
}

func TestDecodedBlockCacheSeparatesTypeKeys(t *testing.T) {
	// Store a block and create the shared cache for type-key isolation.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "type-boundary"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Populate the cache with the first decoded block type.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	_, first := block.NewTransaction(store, nil, ref, nil)
	if _, err := first.Unmarshal(opCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	// Decode the same reference as the alternate block type.
	_, second := block.NewTransaction(store, nil, ref, nil)
	secondBlock, err := second.Unmarshal(opCtx, func() block.Block { return &alternateExample{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := secondBlock.(*alternateExample).msg; got != "type-boundary" {
		t.Fatalf("decoded alternate message = %q, want type-boundary", got)
	}

	// Verify distinct type keys produce independent cache misses.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 2 ||
		snapshot.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("different decoded block type keys should not collide: %+v", snapshot)
	}
}

func TestDecodedBlockCacheRejectsEntriesOverBudget(t *testing.T) {
	// Create a decoded-block cache whose budget cannot retain the block.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "over-budget"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{
		MaxCost: 1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	if decodedBlocks.MaxCost() != 1 {
		t.Fatalf("MaxCost = %d, want 1", decodedBlocks.MaxCost())
	}

	// Decode the over-budget block twice through the shared cache.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	for range 2 {
		_, cursor := block.NewTransaction(store, nil, ref, nil)
		blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got := blk.(*block_mock.Example).GetMsg(); got != "over-budget" {
			t.Fatalf("decoded message = %q, want over-budget", got)
		}
		decodedBlocks.Wait()
	}

	// Verify over-budget entries preserve reads and remain within the cache budget.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 2 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockStoreAttemptCount != 2 ||
		snapshot.DecodedBlockStoreCost == 0 {
		t.Fatalf("over-budget entries should not change reads: %+v", snapshot)
	}
	cacheSnapshot := decodedBlocks.Snapshot()
	if cacheSnapshot.MaxCost != 1 || cacheSnapshot.RetainedCost > 1 {
		t.Fatalf("unexpected cache snapshot after over-budget stores: %+v", cacheSnapshot)
	}
}

func TestDecodedBlockCacheUsesDecodedEntryCostAboveRawBytes(t *testing.T) {
	// Create the block whose decoded cost exceeds its raw-byte size.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	want := &block_mock.Example{Msg: "decoded-cost"}
	rawData, err := want.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the measured block and create a cache with its raw-byte budget.
	ref, _, err := block.PutBlock(ctx, store, want)
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{
		MaxCost: int64(len(rawData)),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Decode the block twice through the undersized cache.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	for range 2 {
		_, cursor := block.NewTransaction(store, nil, ref, nil)
		blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got := blk.(*block_mock.Example).GetMsg(); got != "decoded-cost" {
			t.Fatalf("decoded message = %q, want decoded-cost", got)
		}
		decodedBlocks.Wait()
	}

	// Verify decoded entry cost prevents retaining the block.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockStoreCost <= uint64(len(rawData))*2 {
		t.Fatalf("decoded-entry cost should exceed raw-byte budget: %+v", snapshot)
	}
	cacheSnapshot := decodedBlocks.Snapshot()
	if cacheSnapshot.RetainedCost != 0 || cacheSnapshot.Stores != 0 {
		t.Fatalf("decoded-entry cost should not retain over-budget entries: counter=%+v cache=%+v", snapshot, cacheSnapshot)
	}
}

func TestDecodedBlockCacheDisabledDoesNotRetain(t *testing.T) {
	// Store a block and create a disabled decoded-block cache.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "disabled"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{
		Disabled: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Decode the block twice with shared caching disabled.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	for range 2 {
		_, cursor := block.NewTransaction(store, nil, ref, nil)
		blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got := blk.(*block_mock.Example).GetMsg(); got != "disabled" {
			t.Fatalf("decoded message = %q, want disabled", got)
		}
	}

	// Verify the disabled cache records no shared cache attempts.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheAttemptCount != 0 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockStoreAttemptCount != 0 {
		t.Fatalf("disabled cache should not retain or count shared attempts: %+v", snapshot)
	}
}

func TestDecodedBlockCacheInvalidateRefRemovesSharedEntries(t *testing.T) {
	// Store a block and create the shared cache for invalidation.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "removed"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Populate the shared cache before removing the stored block.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	_, first := block.NewTransaction(store, nil, ref, nil)
	if _, err := first.Unmarshal(opCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	// Remove the stored block and invalidate its shared cache entry.
	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.InvalidateRef(opCtx, ref)
	_, second := block.NewTransaction(store, nil, ref, nil)
	if _, err := second.Unmarshal(opCtx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after invalidation error = %v, want %v", err, block.ErrNotFound)
	}

	// Verify the invalidated reference causes a fresh storage miss.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("invalidated ref should miss shared cache after removal: %+v", snapshot)
	}
}

type corruptingStore struct {
	block.StoreOps
	data []byte
}

func (s *corruptingStore) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return append([]byte(nil), s.data...), true, nil
}

type passthroughTransformer struct{}

func (passthroughTransformer) EncodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

func (passthroughTransformer) DecodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

type alternateExample struct {
	msg string
}

func (e *alternateExample) DecodedBlockCacheTypeKey() string {
	return "db/block/mock.AlternateExample"
}

func (e *alternateExample) MarshalBlock() ([]byte, error) {
	return (&block_mock.Example{Msg: e.msg}).MarshalBlock()
}

func (e *alternateExample) UnmarshalBlock(data []byte) error {
	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(data); err != nil {
		return err
	}
	e.msg = example.GetMsg()
	return nil
}

func (e *alternateExample) CloneBlock() (block.Block, error) {
	return &alternateExample{msg: e.msg}, nil
}
