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
	// Store an example block and retain a lifecycle decoded-block cache.
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

	// Attach the lifecycle cache to a cursor at the example root.
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

	// Read and mutate the first decoded example while the cache retains its value.
	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursor.Unmarshal(firstCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	first.(*block_mock.Example).Msg = "mutated"
	decodedBlocks.Wait()

	// Clone the cursor while retaining the shared lifecycle cache.
	secondCursor := cursor.Clone()
	defer secondCursor.Release()

	// Read the example through the cloned cursor.
	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the cache returns the original example after caller mutation.
	if got := second.(*block_mock.Example).GetMsg(); got != "resource-cache" {
		t.Fatalf("lifecycle cache clone msg = %q, want resource-cache", got)
	}

	// Verify the first read fetched and decoded the example once.
	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first decoded cache counters: %+v", firstSnapshot)
	}

	// Verify the cloned cursor reads a copied example from the cache.
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second decoded cache counters: %+v", secondSnapshot)
	}

	// Follow the example reference while retaining the lifecycle cache.
	thirdCursor, err := cursor.FollowRef(ctx, &bucket.ObjectRef{RootRef: ref})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer thirdCursor.Release()
	thirdCtx, thirdCounter := block.WithReadCounter(ctx)
	if _, err := thirdCursor.Unmarshal(thirdCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the followed cursor reads the retained decoded example.
	thirdSnapshot := thirdCounter.Snapshot()
	if thirdSnapshot.BlockReadCount != 0 ||
		thirdSnapshot.DecodedBlockCacheHitCount != 1 {
		t.Fatalf("followed cursor should borrow lifecycle cache: %+v", thirdSnapshot)
	}

	// Release the original cursor and attach a fresh cursor to the lifecycle cache.
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

	// Verify releasing the original cursor preserved the shared cache.
	fourthSnapshot := fourthCounter.Snapshot()
	if fourthSnapshot.BlockReadCount != 0 ||
		fourthSnapshot.DecodedBlockCacheHitCount != 1 {
		t.Fatalf("cursor release should not close lifecycle cache: %+v", fourthSnapshot)
	}
}

func TestCursorBuildTransactionUsesBucketDefaultPutOpts(t *testing.T) {
	// Construct a bucket cursor whose default write options use SHA256.
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

	// Verify transactions inherit the bucket's default write options.
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
	// Prepare the source store and production gzip transformer.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	transformConf, xfrm := newProductionTransform(t, &transform_gzip.Config{})

	// Write a transformed example and retain its lifecycle decoded-block cache.
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

	// Attach the transform-aware cache to a cursor at the encoded example root.
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

	// Read and mutate the transformed example before cache publication completes.
	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursor.Unmarshal(firstCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	first.(*block_mock.Example).Msg = "mutated"
	decodedBlocks.Wait()

	// Clone the transformed cursor while retaining its lifecycle cache.
	secondCursor := cursor.Clone()
	defer secondCursor.Release()

	// Read the transformed example through the cloned cursor.
	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the cached transformed example is isolated from caller mutation.
	if got := second.(*block_mock.Example).GetMsg(); got != "transformed-resource-cache" {
		t.Fatalf("transformed lifecycle cache clone msg = %q, want transformed-resource-cache", got)
	}

	// Verify the first transformed read populated the decoded-block cache.
	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 ||
		firstSnapshot.DecodedBlockStoreAcceptedCount != 1 ||
		firstSnapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected first transformed decoded cache counters: %+v", firstSnapshot)
	}

	// Verify the second transformed read clones the cached example.
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
	// Store an example block and retain a lifecycle cache for its transactions.
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

	// Attach the lifecycle cache to the example's bucket cursor.
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

	// Read the example through the first block transaction.
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

	// Read the same example through a second block transaction.
	secondCtx, secondCounter := block.WithReadCounter(ctx)
	_, secondCursor := cursor.BuildTransaction(nil)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "transaction-cache" {
		t.Fatalf("second decoded message = %q, want transaction-cache", got)
	}

	// Verify the first transaction fetched and decoded the stored example.
	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first transaction counters: %+v", firstSnapshot)
	}

	// Verify the second transaction reused a cloned cached example.
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
	// Store an example and construct a cursor without a lifecycle cache.
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

	// Read the example through the first uncached cursor.
	firstCtx, firstCounter := block.WithReadCounter(ctx)
	if _, err := cursor.Unmarshal(firstCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}

	// Read the same example through a separate uncached cursor.
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

	// Verify the first uncached read fetched and decoded the example.
	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 0 ||
		firstSnapshot.DecodedBlockUncacheableCount != 1 {
		t.Fatalf("unexpected first uncached counters: %+v", firstSnapshot)
	}

	// Verify the second uncached read fetched and decoded the example again.
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 1 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("unexpected second uncached counters: %+v", secondSnapshot)
	}
}

func TestLifecycleDecodedCacheRepeatedReadWorkloadCounters(t *testing.T) {
	// Store an example for the repeated-read workload.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "workload"})
	if err != nil {
		t.Fatal(err.Error())
	}
	const operations = 8

	// Verify repeated reads without a cache fetch and decode every time.
	uncached := runRepeatedUnmarshalWorkload(t, ctx, store, ref, nil, operations)
	if uncached.BlockReadCount != operations ||
		uncached.DecodedBlockUnmarshalCount != operations ||
		uncached.DecodedBlockCacheHitCount != 0 ||
		uncached.DecodedBlockUncacheableCount != operations {
		t.Fatalf("uncached workload counters: %+v", uncached)
	}

	// Retain a decoded-block cache for the repeated-read workload.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Verify repeated cached reads fetch and decode the example once.
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

	// Verify the lifecycle cache retains decoded data within its cost budget.
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
			// Construct an example cursor borrowing the supplied lifecycle cache.
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

			// Read the example with counters for this workload operation.
			opCtx, counter := block.WithReadCounter(ctx)
			blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Verify the workload read decoded the expected example.
			if got := blk.(*block_mock.Example).GetMsg(); got != "workload" {
				t.Fatalf("decoded message = %q, want workload", got)
			}

			// Wait for publication of this operation's decoded cache entry.
			if decodedBlocks != nil {
				decodedBlocks.Wait()
			}

			// Accumulate block reads and decoding work for the workload.
			snapshot := counter.Snapshot()
			total.BlockReadCount += snapshot.BlockReadCount
			total.DecodedBlockUnmarshalCount += snapshot.DecodedBlockUnmarshalCount

			// Accumulate decoded-cache lookup and clone work for the workload.
			total.DecodedBlockCacheAttemptCount += snapshot.DecodedBlockCacheAttemptCount
			total.DecodedBlockCacheMissCount += snapshot.DecodedBlockCacheMissCount
			total.DecodedBlockCacheHitCount += snapshot.DecodedBlockCacheHitCount
			total.DecodedBlockCloneCount += snapshot.DecodedBlockCloneCount
			total.DecodedBlockUncacheableCount += snapshot.DecodedBlockUncacheableCount

			// Accumulate decoded-cache storage work for the workload.
			total.DecodedBlockStoreAttemptCount += snapshot.DecodedBlockStoreAttemptCount
			total.DecodedBlockStoreAcceptedCount += snapshot.DecodedBlockStoreAcceptedCount
			total.DecodedBlockStoreCost += snapshot.DecodedBlockStoreCost
		}()
	}
	return total
}

func newProductionTransform(t *testing.T, steps ...config.Config) (*block_transform.Config, *block_transform.Transformer) {
	// Build the production transform configuration for the requested steps.
	t.Helper()
	transformConf, err := block_transform.NewConfig(steps)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Construct a transformer with the production step factories.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, transform_all.BuildFactorySet(), transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	return transformConf, xfrm
}
