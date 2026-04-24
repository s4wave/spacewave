package block_rpc_server

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	block_rpc "github.com/aperturerobotics/hydra/block/rpc"
)

type testStore struct {
	block.NopStoreOps

	features     block.StoreFeature
	batchEntries []*block.PutBatchEntry
	beginCount   int
	endCount     int
}

func (s *testStore) GetSupportedFeatures() block.StoreFeature {
	return s.features
}

func (s *testStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	s.batchEntries = entries
	return nil
}

func (s *testStore) BeginDeferFlush() {
	s.beginCount++
}

func (s *testStore) EndDeferFlush(context.Context) error {
	s.endCount++
	return nil
}

func TestBlockStoreGetSupportedFeaturesForwards(t *testing.T) {
	store := &testStore{
		features: block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists,
	}
	server := NewBlockStore(store)

	resp, err := server.GetSupportedFeatures(context.Background(), &block_rpc.GetSupportedFeaturesRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := resp.GetFeatures(); got != store.features {
		t.Fatalf("expected features %v, got %v", store.features, got)
	}
}

func TestBlockStorePutBlockBatchForwardsRefs(t *testing.T) {
	store := &testStore{}
	server := NewBlockStore(store)
	ref := &block.BlockRef{}
	outRef := &block.BlockRef{}

	resp, err := server.PutBlockBatch(context.Background(), &block_rpc.PutBlockBatchRequest{
		Entries: []*block_rpc.PutBlockBatchEntry{{
			Ref:  ref,
			Data: []byte("hello"),
			Refs: []*block.BlockRef{outRef},
		}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if errStr := resp.GetError(); errStr != "" {
		t.Fatal(errStr)
	}
	if len(store.batchEntries) != 1 {
		t.Fatalf("expected one entry, got %d", len(store.batchEntries))
	}
	if got := store.batchEntries[0].Refs; len(got) != 1 || got[0] != outRef {
		t.Fatalf("expected refs to forward through batch request")
	}
}

func TestBlockStoreDeferFlushRefCounts(t *testing.T) {
	store := &testStore{}
	server := NewBlockStore(store)
	ctx := context.Background()

	if _, err := server.BeginDeferFlush(ctx, &block_rpc.BeginDeferFlushRequest{}); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := server.BeginDeferFlush(ctx, &block_rpc.BeginDeferFlushRequest{}); err != nil {
		t.Fatal(err.Error())
	}
	if store.beginCount != 1 {
		t.Fatalf("expected one inner begin, got %d", store.beginCount)
	}

	resp, err := server.EndDeferFlush(ctx, &block_rpc.EndDeferFlushRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if errStr := resp.GetError(); errStr != "" {
		t.Fatal(errStr)
	}
	if store.endCount != 0 {
		t.Fatalf("expected no inner end before outermost close, got %d", store.endCount)
	}

	resp, err = server.EndDeferFlush(ctx, &block_rpc.EndDeferFlushRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if errStr := resp.GetError(); errStr != "" {
		t.Fatal(errStr)
	}
	if store.endCount != 1 {
		t.Fatalf("expected one inner end, got %d", store.endCount)
	}
}

func TestBlockStoreEndDeferFlushUnderflow(t *testing.T) {
	store := &testStore{}
	server := NewBlockStore(store)

	resp, err := server.EndDeferFlush(context.Background(), &block_rpc.EndDeferFlushRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetError() == "" {
		t.Fatal("expected underflow error")
	}
	if store.endCount != 0 {
		t.Fatalf("expected no inner end on underflow, got %d", store.endCount)
	}
}

// _ is a type assertion
var _ block.StoreOps = ((*testStore)(nil))
