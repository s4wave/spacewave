package block_rpc_server

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

type testStore struct {
	block.NopStoreOps

	features     block.StoreFeature
	batchEntries []*block.PutBatchEntry
	fenced       bool
	syncCount    int
}

func (s *testStore) GetSupportedFeatures() block.StoreFeature {
	return s.features
}

func (s *testStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	s.batchEntries = entries
	return nil
}

func (s *testStore) Sync(context.Context) (bool, error) {
	s.syncCount++
	return s.fenced, nil
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

func TestBlockStoreSyncForwards(t *testing.T) {
	store := &testStore{fenced: true}
	server := NewBlockStore(store)

	resp, err := server.Sync(context.Background(), &block_rpc.SyncRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if errStr := resp.GetError(); errStr != "" {
		t.Fatal(errStr)
	}
	if !resp.GetFenced() {
		t.Fatal("expected fenced result to forward through Sync response")
	}
	if store.syncCount != 1 {
		t.Fatalf("expected one inner Sync, got %d", store.syncCount)
	}
}

// _ is a type assertion
var _ block.StoreOps = (*testStore)(nil)
