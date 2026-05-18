package resource_bucket_lookup

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	want := &block_mock.Example{Msg: "manifest data"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)
	got, err := resource.Unmarshal(ctx, &s4wave_bucket_lookup.UnmarshalRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !got.GetFound() {
		t.Fatal("expected block data to be found")
	}

	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(got.GetData()); err != nil {
		t.Fatal(err.Error())
	}
	if example.GetMsg() != want.GetMsg() {
		t.Fatalf("message = %q, want %q", example.GetMsg(), want.GetMsg())
	}
}

func TestUnmarshalWithBlockTypeReusesResourceDecodedCache(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	want := &block_mock.Example{Msg: "typed resource"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	cursor.SetDecodedBlockCache(decodedBlocks)
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)

	opCtx, counter := block.WithReadCounter(ctx)
	resp, err := resource.Unmarshal(opCtx, &s4wave_bucket_lookup.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed resource")
	decodedBlocks.Wait()

	resp, err = resource.Unmarshal(opCtx, &s4wave_bucket_lookup.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed resource")

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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	want := &block_mock.Example{Msg: "transaction resource"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	cursor.SetDecodedBlockCache(decodedBlocks)

	resourceClient := newRecordingResourceClient(ctx)
	resourceCtx := resource_server.WithResourceClientContext(ctx, resourceClient)
	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)

	buildResp, err := resource.BuildTransaction(resourceCtx, &s4wave_bucket_lookup.BuildTransactionRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursorClient := resourceClient.blockCursorClient(t, buildResp.GetCursorResourceId())

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := cursorClient.Unmarshal(firstCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, first.GetData(), "transaction resource")
	decodedBlocks.Wait()

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

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first transaction resource counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second transaction resource counters: %+v", secondSnapshot)
	}
}

func TestPutBlockBatchUsesCursorBatch(t *testing.T) {
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

	firstData := []byte("first")
	secondData := []byte("second")
	firstRef := testBlockRef(t, firstData)
	secondRef := testBlockRef(t, secondData)
	tombstoneRef := testBlockRef(t, []byte("deleted"))

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

	resp, err := resource.GetBlockExistsBatch(ctx, &s4wave_bucket_lookup.GetBlockExistsBatchRequest{
		Refs: []*block.BlockRef{firstRef, secondRef},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
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
	c.nextID++
	c.muxes[c.nextID] = mux
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	return c.nextID, nil
}

func (c *recordingResourceClient) ReleaseResource(resourceID uint32) bool {
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

var _ resource_server.ResourceClientContext = ((*recordingResourceClient)(nil))

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
