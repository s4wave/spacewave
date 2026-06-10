package bucket_lookup_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/hash"
)

func TestCursorUnmarshalBorrowsLifecycleDecodedCache(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "resource-cache"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{RootRef: ref},
		nil,
		nil,
	)
	cursor.SetDecodedBlockCache(decodedBlocks)
	defer cursor.Release()

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursor.Unmarshal(firstCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	first.(*block_mock.Example).Msg = "mutated"
	decodedBlocks.Wait()

	secondCursor := cursor.Clone()
	defer secondCursor.Release()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "resource-cache" {
		t.Fatalf("lifecycle cache clone msg = %q, want resource-cache", got)
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first decoded cache counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second decoded cache counters: %+v", secondSnapshot)
	}

	thirdCursor, err := cursor.FollowRef(ctx, &bucket.ObjectRef{RootRef: ref})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer thirdCursor.Release()
	thirdCtx, thirdCounter := block.WithReadCounter(ctx)
	if _, err := thirdCursor.Unmarshal(thirdCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	thirdSnapshot := thirdCounter.Snapshot()
	if thirdSnapshot.BlockReadCount != 0 ||
		thirdSnapshot.DecodedBlockCacheHitCount != 1 {
		t.Fatalf("followed cursor should borrow lifecycle cache: %+v", thirdSnapshot)
	}

	cursor.Release()
	fourthCursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{RootRef: ref},
		nil,
		nil,
	)
	fourthCursor.SetDecodedBlockCache(decodedBlocks)
	defer fourthCursor.Release()
	fourthCtx, fourthCounter := block.WithReadCounter(ctx)
	if _, err := fourthCursor.Unmarshal(fourthCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	fourthSnapshot := fourthCounter.Snapshot()
	if fourthSnapshot.BlockReadCount != 0 ||
		fourthSnapshot.DecodedBlockCacheHitCount != 1 {
		t.Fatalf("cursor release should not close lifecycle cache: %+v", fourthSnapshot)
	}
}

func TestCursorBuildTransactionUsesBucketDefaultPutOpts(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(hash.HashType_HashType_BLAKE3)
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		&cursorDefaultPutOptsBucket{
			StoreOps: store,
			conf: &bucket.Config{
				Id:      "test",
				Rev:     1,
				PutOpts: &block.PutOpts{HashType: hash.HashType_HashType_SHA256},
			},
		},
		nil,
		&bucket.ObjectRef{},
		nil,
		nil,
	)
	defer cursor.Release()

	tx, _ := cursor.BuildTransaction(nil)
	if got := tx.GetPutOpts().GetHashType(); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("transaction hash type = %v, want SHA256", got)
	}
}

type cursorDefaultPutOptsBucket struct {
	block.StoreOps
	conf *bucket.Config
}

func (b *cursorDefaultPutOptsBucket) GetBucketConfig() *bucket.Config {
	return b.conf
}

func TestCursorUnmarshalBorrowsTransformAwareLifecycleDecodedCache(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	transformConf, xfrm := newProductionTransform(t, &transform_gzip.Config{})

	tx, writeCursor := block.NewTransaction(store, xfrm, nil, nil)
	writeCursor.SetBlock(&block_mock.Example{Msg: "transformed-resource-cache"}, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		xfrm,
		&bucket.ObjectRef{RootRef: ref, TransformConf: transformConf},
		nil,
		transformConf,
	)
	cursor.SetDecodedBlockCache(decodedBlocks)
	defer cursor.Release()

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursor.Unmarshal(firstCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	first.(*block_mock.Example).Msg = "mutated"
	decodedBlocks.Wait()

	secondCursor := cursor.Clone()
	defer secondCursor.Release()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "transformed-resource-cache" {
		t.Fatalf("transformed lifecycle cache clone msg = %q, want transformed-resource-cache", got)
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 ||
		firstSnapshot.DecodedBlockStoreAcceptedCount != 1 ||
		firstSnapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected first transformed decoded cache counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second transformed decoded cache counters: %+v", secondSnapshot)
	}
}

func TestBuildTransactionBorrowsLifecycleDecodedCache(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "transaction-cache"})
	if err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{RootRef: ref},
		nil,
		nil,
	)
	cursor.SetDecodedBlockCache(decodedBlocks)
	defer cursor.Release()

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	_, firstCursor := cursor.BuildTransaction(nil)
	first, err := firstCursor.Unmarshal(firstCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := first.(*block_mock.Example).GetMsg(); got != "transaction-cache" {
		t.Fatalf("first decoded message = %q, want transaction-cache", got)
	}
	decodedBlocks.Wait()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	_, secondCursor := cursor.BuildTransaction(nil)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "transaction-cache" {
		t.Fatalf("second decoded message = %q, want transaction-cache", got)
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first transaction counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second transaction counters: %+v", secondSnapshot)
	}
}

func TestCursorUnmarshalWithoutOwnerUsesUncachedPath(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "uncached"})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{RootRef: ref},
		nil,
		nil,
	)
	defer cursor.Release()

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	if _, err := cursor.Unmarshal(firstCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	secondCursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{RootRef: ref},
		nil,
		nil,
	)
	defer secondCursor.Release()
	secondCtx, secondCounter := block.WithReadCounter(ctx)
	if _, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 0 ||
		firstSnapshot.DecodedBlockUncacheableCount != 1 {
		t.Fatalf("unexpected first uncached counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 1 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("unexpected second uncached counters: %+v", secondSnapshot)
	}
}

func TestLifecycleDecodedCacheRepeatedReadWorkloadCounters(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "workload"})
	if err != nil {
		t.Fatal(err.Error())
	}
	const operations = 8

	uncached := runRepeatedUnmarshalWorkload(t, ctx, store, ref, nil, operations)
	if uncached.BlockReadCount != operations ||
		uncached.DecodedBlockUnmarshalCount != operations ||
		uncached.DecodedBlockCacheHitCount != 0 ||
		uncached.DecodedBlockUncacheableCount != operations {
		t.Fatalf("uncached workload counters: %+v", uncached)
	}

	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	cached := runRepeatedUnmarshalWorkload(t, ctx, store, ref, decodedBlocks, operations)
	if cached.BlockReadCount != 1 ||
		cached.DecodedBlockUnmarshalCount != 1 ||
		cached.DecodedBlockCacheAttemptCount != operations ||
		cached.DecodedBlockCacheMissCount != 1 ||
		cached.DecodedBlockCacheHitCount != operations-1 ||
		cached.DecodedBlockCloneCount != operations-1 ||
		cached.DecodedBlockStoreAttemptCount != 1 ||
		cached.DecodedBlockStoreAcceptedCount != 1 ||
		cached.DecodedBlockStoreCost == 0 {
		t.Fatalf("cached workload counters: %+v", cached)
	}

	cacheSnapshot := decodedBlocks.Snapshot()
	if cacheSnapshot.MaxCost != block.DefaultDecodedBlockCacheMaxCost ||
		cacheSnapshot.RetainedCost == 0 ||
		cacheSnapshot.RetainedCost > cacheSnapshot.MaxCost ||
		cacheSnapshot.Hits == 0 ||
		cacheSnapshot.Stores == 0 ||
		cacheSnapshot.CostAdded == 0 {
		t.Fatalf("cached workload snapshot: %+v", cacheSnapshot)
	}
}

func runRepeatedUnmarshalWorkload(
	t *testing.T,
	ctx context.Context,
	store bucket.BucketOps,
	ref *block.BlockRef,
	decodedBlocks *block.DecodedBlockCache,
	operations uint64,
) block.ReadCounterSnapshot {
	t.Helper()
	var total block.ReadCounterSnapshot
	for range operations {
		func() {
			cursor := bucket_lookup.NewCursor(
				ctx,
				nil,
				nil,
				nil,
				store,
				nil,
				&bucket.ObjectRef{RootRef: ref},
				nil,
				nil,
			)
			defer cursor.Release()
			if decodedBlocks != nil {
				cursor.SetDecodedBlockCache(decodedBlocks)
			}
			opCtx, counter := block.WithReadCounter(ctx)
			blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
			if err != nil {
				t.Fatal(err.Error())
			}
			if got := blk.(*block_mock.Example).GetMsg(); got != "workload" {
				t.Fatalf("decoded message = %q, want workload", got)
			}
			if decodedBlocks != nil {
				decodedBlocks.Wait()
			}
			snapshot := counter.Snapshot()
			total.BlockReadCount += snapshot.BlockReadCount
			total.DecodedBlockUnmarshalCount += snapshot.DecodedBlockUnmarshalCount
			total.DecodedBlockCacheAttemptCount += snapshot.DecodedBlockCacheAttemptCount
			total.DecodedBlockCacheMissCount += snapshot.DecodedBlockCacheMissCount
			total.DecodedBlockCacheHitCount += snapshot.DecodedBlockCacheHitCount
			total.DecodedBlockCloneCount += snapshot.DecodedBlockCloneCount
			total.DecodedBlockUncacheableCount += snapshot.DecodedBlockUncacheableCount
			total.DecodedBlockStoreAttemptCount += snapshot.DecodedBlockStoreAttemptCount
			total.DecodedBlockStoreAcceptedCount += snapshot.DecodedBlockStoreAcceptedCount
			total.DecodedBlockStoreCost += snapshot.DecodedBlockStoreCost
		}()
	}
	return total
}

func newProductionTransform(t *testing.T, steps ...config.Config) (*block_transform.Config, *block_transform.Transformer) {
	t.Helper()
	transformConf, err := block_transform.NewConfig(steps)
	if err != nil {
		t.Fatal(err.Error())
	}
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, transform_all.BuildFactorySet(), transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	return transformConf, xfrm
}
