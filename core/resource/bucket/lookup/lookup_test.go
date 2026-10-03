package resource_bucket_lookup

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	s4wave_block_cursor "github.com/s4wave/spacewave/sdk/block/cursor"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	"github.com/sirupsen/logrus"
)

const exampleBlockTypeID = "github.com/s4wave/spacewave/db/block/mock.Example"

func TestUnmarshalUsesCursorRefWhenRequestRefEmpty(t *testing.T) {
	// Create a logger for the bucket cursor testbed.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the testbed that stores the bucket block.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Open an empty bucket cursor and arrange its release.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	// Persist an example block as the bucket cursor root.
	want := &block_mock.Example{Msg: "manifest data"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	// Read the bucket root through an empty unmarshal request.
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)
	got, err := resource.Unmarshal(ctx, &s4wave_bucket_lookup.UnmarshalRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !got.GetFound() {
		t.Fatal("expected block data to be found")
	}

	// Verify the bucket response contains the saved example block.
	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(got.GetData()); err != nil {
		t.Fatal(err.Error())
	}
	if example.GetMsg() != want.GetMsg() {
		t.Fatalf("message = %q, want %q", example.GetMsg(), want.GetMsg())
	}
}

func TestUnmarshalWithBlockTypeReusesResourceDecodedCache(t *testing.T) {
	// Create a logger for typed bucket readback.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the bucket testbed and register the example block type.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	// Open an empty bucket cursor and arrange its release.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	// Persist an example block for typed bucket readback.
	want := &block_mock.Example{Msg: "typed resource"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	// Attach a decoded block cache to the bucket cursor Resource.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	cursor.SetDecodedBlockCache(decodedBlocks)
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)

	// Read and cache the typed bucket block with counters.
	opCtx, counter := block.WithReadCounter(ctx)
	resp, err := resource.Unmarshal(opCtx, &s4wave_bucket_lookup.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed resource")
	decodedBlocks.Wait()

	// Read the typed bucket block again through the decoded cache.
	resp, err = resource.Unmarshal(opCtx, &s4wave_bucket_lookup.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed resource")

	// Verify typed bucket readback uses one decode and one cache hit.
	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 ||
		snapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected typed resource counters: %+v", snapshot)
	}
}

func TestBuildTransactionResourceCursorBorrowsDecodedCache(t *testing.T) {
	// Create a logger for transaction cursor cache readback.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the bucket testbed and register the example block type.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	// Open an empty bucket cursor and arrange its release.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	// Persist an example block for transaction cursor readback.
	want := &block_mock.Example{Msg: "transaction resource"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	// Attach a decoded block cache to the bucket cursor.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	cursor.SetDecodedBlockCache(decodedBlocks)

	// Create the Resource client that retains transaction cursors.
	resourceClient := newRecordingResourceClient(ctx)
	resourceCtx := resource_server.WithResourceClientContext(ctx, resourceClient)
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)

	// Build a transaction Resource and attach its root cursor client.
	buildResp, err := resource.BuildTransaction(resourceCtx, &s4wave_bucket_lookup.BuildTransactionRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursorClient := resourceClient.blockCursorClient(t, buildResp.GetCursorResourceId())

	// Read the example through the first transaction cursor and warm the cache.
	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursorClient.Unmarshal(firstCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, first.GetData(), "transaction resource")
	decodedBlocks.Wait()

	// Build another transaction cursor and read the cached example.
	buildResp, err = resource.BuildTransaction(resourceCtx, &s4wave_bucket_lookup.BuildTransactionRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursorClient = resourceClient.blockCursorClient(t, buildResp.GetCursorResourceId())
	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := cursorClient.Unmarshal(secondCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, second.GetData(), "transaction resource")

	// Verify the first transaction reads and decodes the bucket block.
	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first transaction resource counters: %+v", firstSnapshot)
	}

	// Verify the second transaction borrows the decoded block cache.
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second transaction resource counters: %+v", secondSnapshot)
	}
}

func TestBuildTransactionResourceCursorBorrowsTransformedDecodedCache(t *testing.T) {
	// Create a logger for transformed transaction cursor readback.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the bucket testbed and register the example block type.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	// Open a bucket cursor with a gzip transform and arrange its release.
	transformConf := newResourceTransformConfig(t, &transform_gzip.Config{})
	cursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		le,
		tb.StepFactorySet,
		tb.BucketId,
		tb.Volume.GetID(),
		transformConf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	// Persist an example block through the transformed bucket cursor.
	want := &block_mock.Example{Msg: "transformed transaction resource"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	// Attach a decoded block cache to the transformed bucket cursor.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	cursor.SetDecodedBlockCache(decodedBlocks)

	// Create the Resource client that retains transformed transaction cursors.
	resourceClient := newRecordingResourceClient(ctx)
	resourceCtx := resource_server.WithResourceClientContext(ctx, resourceClient)
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)

	// Build a transaction Resource and attach its root cursor client.
	buildResp, err := resource.BuildTransaction(resourceCtx, &s4wave_bucket_lookup.BuildTransactionRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursorClient := resourceClient.blockCursorClient(t, buildResp.GetCursorResourceId())

	// Read the transformed example and warm the decoded cache.
	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursorClient.Unmarshal(firstCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, first.GetData(), "transformed transaction resource")
	decodedBlocks.Wait()

	// Build another transaction cursor and read the cached transformed example.
	buildResp, err = resource.BuildTransaction(resourceCtx, &s4wave_bucket_lookup.BuildTransactionRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursorClient = resourceClient.blockCursorClient(t, buildResp.GetCursorResourceId())
	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := cursorClient.Unmarshal(secondCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, second.GetData(), "transformed transaction resource")

	// Verify the first transaction decodes and caches the transformed block.
	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 ||
		firstSnapshot.DecodedBlockStoreAcceptedCount != 1 ||
		firstSnapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected first transformed transaction resource counters: %+v", firstSnapshot)
	}

	// Verify the second transaction reuses the transformed block cache.
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second transformed transaction resource counters: %+v", secondSnapshot)
	}
}

func TestGetRefReturnsCursorOpArgs(t *testing.T) {
	// Create a logger for bucket reference readback.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the testbed that supplies the bucket reference.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Open a transformed bucket cursor with a device mirror override.
	transformConf := newResourceTransformConfig(t, &transform_gzip.Config{})
	cursor, err := bucket_lookup.BuildCursor(
		ctx,
		tb.Bus,
		le,
		tb.StepFactorySet,
		tb.Volume.GetID(),
		&bucket.ObjectRef{BucketId: tb.BucketId},
		transformConf,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)
	if !cursor.GetRef().GetTransformConf().GetEmpty() {
		t.Fatal("test cursor unexpectedly stores transform config in raw ref")
	}
	cursor.SetBucketIDOverride("device-mirror")

	// Verify the Resource reference preserves bucket operation arguments.
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)
	resp, err := resource.GetRef(ctx, &s4wave_bucket_lookup.GetRefRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := resp.GetRef()
	if ref.GetBucketId() != tb.BucketId {
		t.Fatalf("bucket id = %q, want %q", ref.GetBucketId(), tb.BucketId)
	}
	if !ref.GetTransformConf().EqualVT(transformConf) {
		t.Fatal("resource ref did not preserve cursor transform config")
	}
	if got := resp.GetBucketIdOverride(); got != "device-mirror" {
		t.Fatalf("bucket override = %q, want device-mirror", got)
	}
}

func TestGetRefPreservesEmptyCursorOpArgs(t *testing.T) {
	// Create an empty bucket cursor with transformation arguments.
	ctx := context.Background()
	transformConf := newResourceTransformConfig(t, &transform_gzip.Config{})
	cursor := bucket_lookup.NewCursorWithRelease(
		ctx,
		nil,
		nil,
		nil,
		block_mock.NewMockStore(0),
		nil,
		nil,
		&bucket.BucketOpArgs{BucketId: "test"},
		transformConf,
		nil,
	)
	resource := NewBucketLookupCursorResource(nil, nil, cursor)

	// Verify the empty cursor Resource preserves its bucket operation arguments.
	resp, err := resource.GetRef(ctx, &s4wave_bucket_lookup.GetRefRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := resp.GetRef()
	if !ref.GetRootRef().GetEmpty() {
		t.Fatalf("root ref = %s, want empty", ref.GetRootRef().MarshalString())
	}
	if ref.GetBucketId() != "test" {
		t.Fatalf("bucket id = %q, want test", ref.GetBucketId())
	}
	if !ref.GetTransformConf().EqualVT(transformConf) {
		t.Fatal("empty cursor ref did not preserve cursor transform config")
	}
}

func TestPutBlockBatchUsesCursorBatch(t *testing.T) {
	// Create a bucket cursor Resource with recorded batch operations.
	ctx := context.Background()
	store := &recordingBucketOps{StoreOps: block_mock.NewMockStore(0)}
	cursor := bucket_lookup.NewCursorWithRelease(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{BucketId: "test"},
		&bucket.BucketOpArgs{BucketId: "test"},
		nil,
		nil,
	)
	resource := NewBucketLookupCursorResource(nil, nil, cursor)

	// Prepare block data, an outgoing reference, and a tombstone for the batch.
	firstData := []byte("first")
	secondData := []byte("second")
	firstRef := testBlockRef(t, firstData)
	secondRef := testBlockRef(t, secondData)
	tombstoneRef := testBlockRef(t, []byte("deleted"))

	// Write the block batch through the bucket cursor Resource.
	_, err := resource.PutBlockBatch(ctx, &s4wave_bucket_lookup.PutBlockBatchRequest{
		Entries: []*s4wave_bucket_lookup.PutBlockBatchEntry{
			{Ref: firstRef, Data: firstData},
			{Ref: secondRef, Data: secondData, Refs: []*block.BlockRef{firstRef}},
			{Ref: tombstoneRef, Tombstone: true},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the Resource forwards one batch with its data, references, and tombstone.
	if store.putBlockCalls != 0 {
		t.Fatalf("PutBlock calls = %d, want 0", store.putBlockCalls)
	}
	if store.putBatchCalls != 1 {
		t.Fatalf("PutBlockBatch calls = %d, want 1", store.putBatchCalls)
	}
	if len(store.putBatchEntries) != 3 {
		t.Fatalf("batch entries = %d, want 3", len(store.putBatchEntries))
	}
	if !bytes.Equal(store.putBatchEntries[0].Data, firstData) {
		t.Fatalf("entry[0] data = %q, want %q", store.putBatchEntries[0].Data, firstData)
	}
	if !store.putBatchEntries[1].Refs[0].EqualVT(firstRef) {
		t.Fatal("entry[1] refs did not preserve outgoing ref")
	}
	if !store.putBatchEntries[2].Tombstone || !store.putBatchEntries[2].Ref.EqualVT(tombstoneRef) {
		t.Fatal("entry[2] did not preserve tombstone")
	}
}

func TestGetBlockExistsBatchUsesCursorBatch(t *testing.T) {
	// Create a bucket cursor Resource with recorded existence batches.
	ctx := context.Background()
	firstRef := testBlockRef(t, []byte("first"))
	secondRef := testBlockRef(t, []byte("second"))
	store := &recordingBucketOps{
		StoreOps:          block_mock.NewMockStore(0),
		existsBatchResult: []bool{true, false},
	}
	cursor := bucket_lookup.NewCursorWithRelease(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{BucketId: "test"},
		&bucket.BucketOpArgs{BucketId: "test"},
		nil,
		nil,
	)
	resource := NewBucketLookupCursorResource(nil, nil, cursor)

	// Query block existence through the bucket cursor Resource.
	resp, err := resource.GetBlockExistsBatch(ctx, &s4wave_bucket_lookup.GetBlockExistsBatchRequest{
		Refs: []*block.BlockRef{firstRef, secondRef},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the existence batch forwards references and returns the recorded results.
	if store.existsBatchCalls != 1 {
		t.Fatalf("GetBlockExistsBatch calls = %d, want 1", store.existsBatchCalls)
	}
	if len(store.existsBatchRefs) != 2 || !store.existsBatchRefs[0].EqualVT(firstRef) {
		t.Fatal("existence batch refs were not forwarded")
	}
	if got := resp.GetFound(); len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("found = %v, want [true false]", got)
	}
}

type recordingBucketOps struct {
	block.StoreOps

	putBlockCalls     int
	putBatchCalls     int
	putBatchEntries   []*block.PutBatchEntry
	existsBatchCalls  int
	existsBatchRefs   []*block.BlockRef
	existsBatchResult []bool
}

type recordingResourceClient struct {
	ctx      context.Context
	nextID   uint32
	muxes    map[uint32]srpc.Invoker
	values   map[uint32]any
	releases map[uint32]func()
}

func newRecordingResourceClient(ctx context.Context) *recordingResourceClient {
	return &recordingResourceClient{
		ctx:      ctx,
		muxes:    make(map[uint32]srpc.Invoker),
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
	}
}

func (c *recordingResourceClient) Context() context.Context {
	return c.ctx
}

func (c *recordingResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *recordingResourceClient) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	// Retain the Resource mux, value, and release callback under a new identifier.
	c.nextID++
	c.muxes[c.nextID] = mux
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	return c.nextID, nil
}

func (c *recordingResourceClient) ReleaseResource(resourceID uint32) bool {
	// Release the retained Resource and remove its recorded registrations.
	releaseFn, ok := c.releases[resourceID]
	if !ok {
		return false
	}
	delete(c.muxes, resourceID)
	delete(c.values, resourceID)
	delete(c.releases, resourceID)
	if releaseFn != nil {
		releaseFn()
	}
	return true
}

func (c *recordingResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	value := c.values[resourceID]
	if value == nil {
		return nil, errors.New("resource value not found")
	}
	return value, nil
}

func (c *recordingResourceClient) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	mux := c.muxes[resourceID]
	if mux == nil {
		return nil, errors.New("resource mux not found")
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))), nil
}

func (c *recordingResourceClient) blockCursorClient(t *testing.T, resourceID uint32) s4wave_block_cursor.SRPCBlockCursorResourceServiceClient {
	t.Helper()
	client, err := c.GetAttachedResource(resourceID)
	if err != nil {
		t.Fatal(err.Error())
	}
	return s4wave_block_cursor.NewSRPCBlockCursorResourceServiceClient(client)
}

var _ resource_server.ResourceClientContext = (*recordingResourceClient)(nil)

func (s *recordingBucketOps) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putBlockCalls++
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *recordingBucketOps) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	s.putBatchCalls++
	s.putBatchEntries = entries
	return nil
}

func (s *recordingBucketOps) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	s.existsBatchRefs = refs
	return s.existsBatchResult, nil
}

func testBlockRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

func addExampleBlockTypeController(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	// Register an example block type controller for bucket decoding.
	t.Helper()
	controller := blocktype_controller.NewController(func(ctx context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == exampleBlockTypeID {
			return blocktype.NewBlockType(exampleBlockTypeID, func() *block_mock.Example {
				return &block_mock.Example{}
			}), nil
		}
		return nil, nil
	})
	release, err := tb.Bus.AddController(ctx, controller, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(release)
}

func newResourceTransformConfig(t *testing.T, steps ...config.Config) *block_transform.Config {
	t.Helper()
	transformConf, err := block_transform.NewConfig(steps)
	if err != nil {
		t.Fatal(err.Error())
	}
	return transformConf
}

func assertExampleResponse(t *testing.T, data []byte, want string) {
	t.Helper()
	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(data); err != nil {
		t.Fatal(err.Error())
	}
	if example.GetMsg() != want {
		t.Fatalf("message = %q, want %q", example.GetMsg(), want)
	}
}

// failingSecondAddResourceClient fails the second AddResource call.
type failingSecondAddResourceClient struct {
	*recordingResourceClient
	adds int
}

func (c *failingSecondAddResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	c.adds++
	if c.adds == 2 {
		return 0, errors.New("add resource failed")
	}
	return c.recordingResourceClient.AddResource(mux, releaseFn)
}

func TestBuildTransactionReleasesTransactionWhenCursorAddFails(t *testing.T) {
	// Create a logger for transaction registration failure checks.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the testbed that supplies the bucket cursor.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Open a bucket cursor Resource and arrange cursor release.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)

	// Verify both transaction builders release registrations after cursor registration fails.
	for name, build := range map[string]func(context.Context) error{
		"BuildTransaction": func(ctx context.Context) error {
			_, err := resource.BuildTransaction(ctx, &s4wave_bucket_lookup.BuildTransactionRequest{})
			return err
		},
		"BuildTransactionAtRef": func(ctx context.Context) error {
			_, err := resource.BuildTransactionAtRef(ctx, &s4wave_bucket_lookup.BuildTransactionAtRefRequest{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			resourceClient := &failingSecondAddResourceClient{recordingResourceClient: newRecordingResourceClient(ctx)}
			if err := build(resource_server.WithResourceClientContext(ctx, resourceClient)); err == nil {
				t.Fatal("expected cursor registration failure")
			}
			if len(resourceClient.muxes) != 0 {
				t.Fatalf("registered resources after failure = %d, want 0", len(resourceClient.muxes))
			}
		})
	}
}
