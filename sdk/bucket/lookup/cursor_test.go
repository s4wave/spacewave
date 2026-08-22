package s4wave_bucket_lookup

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

func TestSDKBucketLookupStorePutBlockBatchUsesRemoteBatch(t *testing.T) {
	ctx := context.Background()
	firstData := []byte("first")
	secondData := []byte("second")
	firstRef := testSDKBlockRef(t, firstData)
	secondRef := testSDKBlockRef(t, secondData)
	tombstoneRef := testSDKBlockRef(t, []byte("deleted"))
	service := &bucketLookupBatchService{}
	store := &cursorStore{service: service}

	err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData, Refs: []*block.BlockRef{firstRef}},
		{Ref: tombstoneRef, Tombstone: true},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if service.putBlockCalls != 0 {
		t.Fatalf("PutBlock calls = %d, want 0", service.putBlockCalls)
	}
	if service.putBatchCalls != 1 {
		t.Fatalf("PutBlockBatch calls = %d, want 1", service.putBatchCalls)
	}
	entries := service.putBatchRequest.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("batch entries = %d, want 3", len(entries))
	}
	if !bytes.Equal(entries[0].GetData(), firstData) {
		t.Fatalf("entry[0] data = %q, want %q", entries[0].GetData(), firstData)
	}
	if !entries[1].GetRefs()[0].EqualVT(firstRef) {
		t.Fatal("entry[1] refs did not preserve outgoing ref")
	}
	if !entries[2].GetTombstone() || !entries[2].GetRef().EqualVT(tombstoneRef) {
		t.Fatal("entry[2] did not preserve tombstone")
	}
}

func TestSDKBucketLookupStoreGetBlockExistsBatchUsesRemoteBatch(t *testing.T) {
	ctx := context.Background()
	firstRef := testSDKBlockRef(t, []byte("first"))
	secondRef := testSDKBlockRef(t, []byte("second"))
	service := &bucketLookupBatchService{
		existsBatchResponse: &GetBlockExistsBatchResponse{
			Found: []bool{true, false},
		},
	}
	store := &cursorStore{service: service}

	found, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{firstRef, secondRef})
	if err != nil {
		t.Fatal(err.Error())
	}
	if service.getBlockCalls != 0 {
		t.Fatalf("GetBlock calls = %d, want 0", service.getBlockCalls)
	}
	if service.existsBatchCalls != 1 {
		t.Fatalf("GetBlockExistsBatch calls = %d, want 1", service.existsBatchCalls)
	}
	if len(service.existsBatchRequest.GetRefs()) != 2 || !service.existsBatchRequest.GetRefs()[0].EqualVT(firstRef) {
		t.Fatal("existence batch refs were not forwarded")
	}
	if len(found) != 2 || !found[0] || found[1] {
		t.Fatalf("found = %v, want [true false]", found)
	}
}

func TestSDKBucketLookupStoreGetBlockRecordsResourceCounter(t *testing.T) {
	ctx := context.Background()
	data := []byte("resource block data")
	ref := testSDKBlockRef(t, data)
	service := &bucketLookupBatchService{
		getBlockResponse: &GetBlockResponse{
			Data:  data,
			Found: true,
		},
	}
	store := &cursorStore{service: service}

	opCtx, counter := block.WithReadCounter(ctx)
	got, found, err := store.GetBlock(opCtx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("GetBlock found=%v data=%q, want true %q", found, got, data)
	}
	if service.getBlockCalls != 1 {
		t.Fatalf("GetBlock calls = %d, want 1", service.getBlockCalls)
	}
	snapshot := counter.Snapshot()
	if snapshot.ResourceGetBlockCount != 1 ||
		snapshot.ResourceGetBlockRefCount != 1 ||
		snapshot.ResourceGetBlockBytes != uint64(len(data)) ||
		snapshot.ResourceGetBlockMissCount != 0 {
		t.Fatalf("unexpected resource GetBlock counters: %+v", snapshot)
	}
}

func TestSDKBucketLookupStoreReadOperationReusesDecodedBlocks(t *testing.T) {
	ctx := context.Background()
	encoded, err := (&block_mock.Example{Msg: "resource decoded"}).MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := testSDKBlockRef(t, encoded)
	service := &bucketLookupBatchService{
		getBlockResponse: &GetBlockResponse{
			Data:  encoded,
			Found: true,
		},
	}
	store := &cursorStore{service: service}
	_, first := block.NewTransaction(store, nil, ref, nil)
	_, second := block.NewTransaction(store, nil, ref, nil)
	ctor := func() block.Block { return &block_mock.Example{} }

	opCtx, counter := block.WithReadCounter(ctx)
	scopedStore, release, err := store.BeginReadOperation(opCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	opCtx = block.WithReadOperationStore(opCtx, scopedStore)

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
	if secondExample.GetMsg() != "resource decoded" {
		t.Fatalf("cached resource block message = %q, want resource decoded", secondExample.GetMsg())
	}
	if firstExample == secondExample {
		t.Fatal("resource cache hit returned the first decoded block instance")
	}
	if service.getBlockCalls != 1 {
		t.Fatalf("GetBlock calls = %d, want 1", service.getBlockCalls)
	}
	snapshot := counter.Snapshot()
	if snapshot.ResourceGetBlockCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 ||
		snapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected resource decoded cache counters: %+v", snapshot)
	}
}

type bucketLookupBatchService struct {
	putBlockCalls       int
	putBatchCalls       int
	putBatchRequest     *PutBlockBatchRequest
	getBlockCalls       int
	getBlockResponse    *GetBlockResponse
	existsBatchCalls    int
	existsBatchRequest  *GetBlockExistsBatchRequest
	existsBatchResponse *GetBlockExistsBatchResponse
}

func (s *bucketLookupBatchService) SRPCClient() srpc.Client { return nil }

func (s *bucketLookupBatchService) GetRef(context.Context, *GetRefRequest) (*GetRefResponse, error) {
	return nil, errors.New("unexpected GetRef")
}

func (s *bucketLookupBatchService) FollowRef(context.Context, *FollowRefRequest) (*FollowRefResponse, error) {
	return nil, errors.New("unexpected FollowRef")
}

func (s *bucketLookupBatchService) GetBlock(context.Context, *GetBlockRequest) (*GetBlockResponse, error) {
	s.getBlockCalls++
	if s.getBlockResponse != nil {
		return s.getBlockResponse, nil
	}
	return nil, errors.New("unexpected GetBlock")
}

func (s *bucketLookupBatchService) PutBlock(context.Context, *PutBlockRequest) (*PutBlockResponse, error) {
	s.putBlockCalls++
	return nil, errors.New("unexpected PutBlock")
}

func (s *bucketLookupBatchService) PutBlockBatch(_ context.Context, req *PutBlockBatchRequest) (*PutBlockBatchResponse, error) {
	s.putBatchCalls++
	s.putBatchRequest = req
	return &PutBlockBatchResponse{}, nil
}

func (s *bucketLookupBatchService) GetBlockExistsBatch(_ context.Context, req *GetBlockExistsBatchRequest) (*GetBlockExistsBatchResponse, error) {
	s.existsBatchCalls++
	s.existsBatchRequest = req
	if s.existsBatchResponse != nil {
		return s.existsBatchResponse, nil
	}
	return &GetBlockExistsBatchResponse{}, nil
}

func (s *bucketLookupBatchService) BuildTransaction(context.Context, *BuildTransactionRequest) (*BuildTransactionResponse, error) {
	return nil, errors.New("unexpected BuildTransaction")
}

func (s *bucketLookupBatchService) BuildTransactionAtRef(context.Context, *BuildTransactionAtRefRequest) (*BuildTransactionAtRefResponse, error) {
	return nil, errors.New("unexpected BuildTransactionAtRef")
}

func (s *bucketLookupBatchService) Clone(context.Context, *CloneRequest) (*CloneResponse, error) {
	return nil, errors.New("unexpected Clone")
}

func (s *bucketLookupBatchService) Release(context.Context, *ReleaseRequest) (*ReleaseResponse, error) {
	return nil, errors.New("unexpected Release")
}

func (s *bucketLookupBatchService) Unmarshal(context.Context, *UnmarshalRequest) (*UnmarshalResponse, error) {
	return nil, errors.New("unexpected Unmarshal")
}

func testSDKBlockRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}
