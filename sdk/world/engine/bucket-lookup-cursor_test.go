package sdk_world_engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
)

func TestSDKBucketLookupStorePutBlockBatchUsesRemoteBatch(t *testing.T) {
	ctx := context.Background()
	firstData := []byte("first")
	secondData := []byte("second")
	firstRef := testSDKBlockRef(t, firstData)
	secondRef := testSDKBlockRef(t, secondData)
	tombstoneRef := testSDKBlockRef(t, []byte("deleted"))
	service := &bucketLookupBatchService{}
	store := &sdkBucketLookupStore{service: service}

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
		existsBatchResponse: &s4wave_bucket_lookup.GetBlockExistsBatchResponse{
			Found: []bool{true, false},
		},
	}
	store := &sdkBucketLookupStore{service: service}

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

type bucketLookupBatchService struct {
	putBlockCalls       int
	putBatchCalls       int
	putBatchRequest     *s4wave_bucket_lookup.PutBlockBatchRequest
	getBlockCalls       int
	existsBatchCalls    int
	existsBatchRequest  *s4wave_bucket_lookup.GetBlockExistsBatchRequest
	existsBatchResponse *s4wave_bucket_lookup.GetBlockExistsBatchResponse
}

func (s *bucketLookupBatchService) SRPCClient() srpc.Client { return nil }

func (s *bucketLookupBatchService) GetRef(context.Context, *s4wave_bucket_lookup.GetRefRequest) (*s4wave_bucket_lookup.GetRefResponse, error) {
	return nil, errors.New("unexpected GetRef")
}

func (s *bucketLookupBatchService) FollowRef(context.Context, *s4wave_bucket_lookup.FollowRefRequest) (*s4wave_bucket_lookup.FollowRefResponse, error) {
	return nil, errors.New("unexpected FollowRef")
}

func (s *bucketLookupBatchService) GetBlock(context.Context, *s4wave_bucket_lookup.GetBlockRequest) (*s4wave_bucket_lookup.GetBlockResponse, error) {
	s.getBlockCalls++
	return nil, errors.New("unexpected GetBlock")
}

func (s *bucketLookupBatchService) PutBlock(context.Context, *s4wave_bucket_lookup.PutBlockRequest) (*s4wave_bucket_lookup.PutBlockResponse, error) {
	s.putBlockCalls++
	return nil, errors.New("unexpected PutBlock")
}

func (s *bucketLookupBatchService) PutBlockBatch(_ context.Context, req *s4wave_bucket_lookup.PutBlockBatchRequest) (*s4wave_bucket_lookup.PutBlockBatchResponse, error) {
	s.putBatchCalls++
	s.putBatchRequest = req
	return &s4wave_bucket_lookup.PutBlockBatchResponse{}, nil
}

func (s *bucketLookupBatchService) GetBlockExistsBatch(_ context.Context, req *s4wave_bucket_lookup.GetBlockExistsBatchRequest) (*s4wave_bucket_lookup.GetBlockExistsBatchResponse, error) {
	s.existsBatchCalls++
	s.existsBatchRequest = req
	if s.existsBatchResponse != nil {
		return s.existsBatchResponse, nil
	}
	return &s4wave_bucket_lookup.GetBlockExistsBatchResponse{}, nil
}

func (s *bucketLookupBatchService) BuildTransaction(context.Context, *s4wave_bucket_lookup.BuildTransactionRequest) (*s4wave_bucket_lookup.BuildTransactionResponse, error) {
	return nil, errors.New("unexpected BuildTransaction")
}

func (s *bucketLookupBatchService) BuildTransactionAtRef(context.Context, *s4wave_bucket_lookup.BuildTransactionAtRefRequest) (*s4wave_bucket_lookup.BuildTransactionAtRefResponse, error) {
	return nil, errors.New("unexpected BuildTransactionAtRef")
}

func (s *bucketLookupBatchService) Clone(context.Context, *s4wave_bucket_lookup.CloneRequest) (*s4wave_bucket_lookup.CloneResponse, error) {
	return nil, errors.New("unexpected Clone")
}

func (s *bucketLookupBatchService) Release(context.Context, *s4wave_bucket_lookup.ReleaseRequest) (*s4wave_bucket_lookup.ReleaseResponse, error) {
	return nil, errors.New("unexpected Release")
}

func (s *bucketLookupBatchService) Unmarshal(context.Context, *s4wave_bucket_lookup.UnmarshalRequest) (*s4wave_bucket_lookup.UnmarshalResponse, error) {
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
