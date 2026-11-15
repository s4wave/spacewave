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
	putCalls     int
	fenced       bool
	syncCount    int
	// background records whether the last block read was background work.
	background bool
}

func (s *testStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return s.NopStoreOps.PutBlock(ctx, data, opts)
}

func (s *testStore) GetSupportedFeatures() block.StoreFeature {
	return s.features
}

func (s *testStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	s.batchEntries = entries
	return make([]bool, len(entries)), nil
}

func (s *testStore) GetStoredBlock(ctx context.Context, _ *block.BlockRef) (*block.StoredBlock, error) {
	s.background = block.IsBackground(ctx)
	return nil, nil
}

func (s *testStore) Sync(context.Context) (bool, error) {
	s.syncCount++
	return s.fenced, nil
}

func TestBlockStoreGetSupportedFeaturesForwards(t *testing.T) {
	// Prepare a store with native batch feature flags.
	store := &testStore{
		features: block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists,
	}
	server := NewBlockStore(store)

	// Read the supported features through the RPC service.
	resp, err := server.GetSupportedFeatures(context.Background(), &block_rpc.GetSupportedFeaturesRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the response preserves the store feature flags.
	if got := resp.GetFeatures(); got != store.features {
		t.Fatalf("expected features %v, got %v", store.features, got)
	}
}

func TestBlockStorePutBlockBatchForwardsRefs(t *testing.T) {
	// Prepare the batch store and its block references.
	store := &testStore{}
	server := NewBlockStore(store)
	ref := mustBlockRef(t, []byte("hello"))
	outRef := &block.BlockRef{}

	// Send a batch containing block data and outgoing references.
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

	// Verify the batch response and forwarded outgoing references.
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

// TestBlockStorePutBlockBatchRefusesWrongBytes checks that a batch entry whose
// bytes do not hash to its ref is refused with ErrBlockRefMismatch and that
// the store takes nothing of the batch.
func TestBlockStorePutBlockBatchRefusesWrongBytes(t *testing.T) {
	// Send a good entry, then one whose bytes do not match its ref.
	store := &testStore{}
	server := NewBlockStore(store)
	resp, err := server.PutBlockBatch(context.Background(), &block_rpc.PutBlockBatchRequest{
		Entries: []*block_rpc.PutBlockBatchEntry{
			{Ref: mustBlockRef(t, []byte("good")), Data: []byte("good")},
			{Ref: mustBlockRef(t, []byte("named")), Data: []byte("other")},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// The batch is refused before the store sees any entry.
	if got := resp.GetError(); got != block.ErrBlockRefMismatch.Error() {
		t.Fatalf("error = %q, want %q", got, block.ErrBlockRefMismatch.Error())
	}
	if store.batchEntries != nil {
		t.Fatalf("store took %d entries of a refused batch", len(store.batchEntries))
	}
}

// mustBlockRef returns the ref of data.
func mustBlockRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestBlockStoreSyncForwards(t *testing.T) {
	// Prepare a store that reports a durability fence.
	store := &testStore{fenced: true}
	server := NewBlockStore(store)

	// Sync the store through the RPC service.
	resp, err := server.Sync(context.Background(), &block_rpc.SyncRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the service forwards the fence with one store sync.
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

func TestBlockStoreGetBlockRestoresBackground(t *testing.T) {
	store := &testStore{}
	server := NewBlockStore(store)
	for _, background := range []bool{true, false} {
		// Read a block with the request's class.
		_, err := server.GetBlock(context.Background(), &block_rpc.GetBlockRequest{
			Ref:        &block.BlockRef{},
			WithRefs:   true,
			Background: background,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Verify the store read carries the same class.
		if store.background != background {
			t.Fatalf("background request %v reached the store as %v", background, store.background)
		}
	}
}

// TestBlockStoreBackgroundPolicyMarksEveryRead verifies a background service
// marks a read whose request carries no class.
func TestBlockStoreBackgroundPolicyMarksEveryRead(t *testing.T) {
	// Read a block without a class through a background service.
	store := &testStore{}
	server := NewBlockStoreWithPolicy(store, ReadPolicy{Background: true})
	_, err := server.GetBlock(context.Background(), &block_rpc.GetBlockRequest{
		Ref:      &block.BlockRef{},
		WithRefs: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the store read is background work.
	if !store.background {
		t.Fatal("background service read reached the store as foreground")
	}
}

// _ is a type assertion
var _ block.StoreOps = (*testStore)(nil)

// TestBlockStorePutBlockRefusesWrongBytes checks that a single put whose bytes
// do not hash to the ref it forces is refused before the store sees it.
func TestBlockStorePutBlockRefusesWrongBytes(t *testing.T) {
	store := &testStore{}
	server := NewBlockStore(store)
	resp, err := server.PutBlock(context.Background(), &block_rpc.PutBlockRequest{
		Data:    []byte("other"),
		PutOpts: &block.PutOpts{ForceBlockRef: mustBlockRef(t, []byte("named"))},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := resp.GetError(); got != block.ErrBlockRefMismatch.Error() {
		t.Fatalf("error = %q, want %q", got, block.ErrBlockRefMismatch.Error())
	}
	if store.putCalls != 0 {
		t.Fatalf("store took %d puts of refused bytes", store.putCalls)
	}
}
