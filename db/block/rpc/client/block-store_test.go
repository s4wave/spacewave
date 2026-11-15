package block_rpc_client

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
)

// testBlockStoreClient records feature and batch calls without a transport.
type testBlockStoreClient struct {
	features     block.StoreFeature
	featureCalls int
	batchEntries []*block_rpc.PutBlockBatchEntry
	batchHook    func(context.Context, *block_rpc.PutBlockBatchRequest) error
	// existing holds the data of blocks the remote store already has.
	existing map[string]bool
	// shortExisted drops the last existence result from each response.
	shortExisted bool
	// getRequest is the last GetBlock request.
	getRequest *block_rpc.GetBlockRequest
	// getResponse answers GetBlock when set.
	getResponse *block_rpc.GetBlockResponse
}

func (c *testBlockStoreClient) SRPCClient() srpc.Client {
	return nil
}

func (c *testBlockStoreClient) GetHashType(
	context.Context,
	*block_rpc.GetHashTypeRequest,
) (*block_rpc.GetHashTypeResponse, error) {
	return &block_rpc.GetHashTypeResponse{}, nil
}

func (c *testBlockStoreClient) GetSupportedFeatures(
	context.Context,
	*block_rpc.GetSupportedFeaturesRequest,
) (*block_rpc.GetSupportedFeaturesResponse, error) {
	c.featureCalls++
	return &block_rpc.GetSupportedFeaturesResponse{Features: c.features}, nil
}

func (c *testBlockStoreClient) PutBlock(
	context.Context,
	*block_rpc.PutBlockRequest,
) (*block_rpc.PutBlockResponse, error) {
	return &block_rpc.PutBlockResponse{}, nil
}

func (c *testBlockStoreClient) PutBlockBatch(
	ctx context.Context,
	req *block_rpc.PutBlockBatchRequest,
) (*block_rpc.PutBlockBatchResponse, error) {
	// Record the batch and run the test's hook.
	c.batchEntries = req.GetEntries()
	if c.batchHook != nil {
		if err := c.batchHook(ctx, req); err != nil {
			return nil, err
		}
	}

	// Answer from the existing set, one result per entry.
	resp := &block_rpc.PutBlockBatchResponse{Existed: make([]bool, len(req.GetEntries()))}
	for i, entry := range req.GetEntries() {
		resp.Existed[i] = c.existing[string(entry.GetData())]
	}
	if c.shortExisted {
		resp.Existed = resp.Existed[:len(resp.Existed)-1]
	}
	return resp, nil
}

func (c *testBlockStoreClient) GetBlock(
	_ context.Context,
	req *block_rpc.GetBlockRequest,
) (*block_rpc.GetBlockResponse, error) {
	c.getRequest = req
	if c.getResponse != nil {
		return c.getResponse, nil
	}
	return &block_rpc.GetBlockResponse{}, nil
}

func (c *testBlockStoreClient) GetBlockExists(
	context.Context,
	*block_rpc.GetBlockExistsRequest,
) (*block_rpc.GetBlockExistsResponse, error) {
	return &block_rpc.GetBlockExistsResponse{}, nil
}

func (c *testBlockStoreClient) GetBlockExistsBatch(
	context.Context,
	*block_rpc.GetBlockExistsBatchRequest,
) (*block_rpc.GetBlockExistsBatchResponse, error) {
	return &block_rpc.GetBlockExistsBatchResponse{}, nil
}

func (c *testBlockStoreClient) RmBlock(
	context.Context,
	*block_rpc.RmBlockRequest,
) (*block_rpc.RmBlockResponse, error) {
	return &block_rpc.RmBlockResponse{}, nil
}

func (c *testBlockStoreClient) StatBlock(
	context.Context,
	*block_rpc.StatBlockRequest,
) (*block_rpc.StatBlockResponse, error) {
	return &block_rpc.StatBlockResponse{}, nil
}

func (c *testBlockStoreClient) Sync(
	context.Context,
	*block_rpc.SyncRequest,
) (*block_rpc.SyncResponse, error) {
	return &block_rpc.SyncResponse{}, nil
}

func TestBlockStoreGetSupportedFeaturesMasksReadOnlyWrites(t *testing.T) {
	// Configure a remote store that advertises batching and self-buffered features.
	remote := block.StoreFeatureNativeBatchPut |
		block.StoreFeatureNativeBatchExists |
		block.StoreFeatureSelfBuffered

	// Build the read-only block store over the test client.
	client := &testBlockStoreClient{features: remote}
	store := NewBlockStore(client, 0, true)

	// Compare every read against the expected masked feature set.
	expected := block.StoreFeatureNativeBatchExists | block.StoreFeatureSelfBuffered

	// Concurrent first readers must wait for the same initialized feature set.
	var readers sync.WaitGroup
	for range 16 {
		readers.Go(func() {
			if got := store.GetSupportedFeatures(); got != expected {
				t.Errorf("expected read-safe features on read-only client, got %v", got)
			}
		})
	}
	readers.Wait()

	// Read the cached feature set once more after all concurrent readers finish.
	got := store.GetSupportedFeatures()
	if got != expected {
		t.Fatalf("expected cached read-only feature set, got %v", got)
	}

	// Assert the remote was queried exactly once despite the concurrent readers.
	if client.featureCalls != 1 {
		t.Fatalf("expected one feature RPC call, got %d", client.featureCalls)
	}
}

func TestBlockStorePutBlockBatchForwardsRefs(t *testing.T) {
	// Build a write-through block store and batch entry with one forwarded ref.
	client := &testBlockStoreClient{}
	store := NewBlockStore(client, 0, false)
	ref := &block.BlockRef{}
	outRef := &block.BlockRef{}

	// Send one batch entry carrying the ref and payload through the store.
	if _, err := store.PutBlockBatch(context.Background(), []*block.PutBatchEntry{{
		Ref:  ref,
		Data: []byte("hello"),
		Refs: []*block.BlockRef{outRef},
	}}); err != nil {
		t.Fatal(err.Error())
	}

	// Assert the batch request carried exactly one entry.
	if len(client.batchEntries) != 1 {
		t.Fatalf("expected one batch entry, got %d", len(client.batchEntries))
	}

	// Assert the block refs were forwarded verbatim through the batch request.
	if got := client.batchEntries[0].GetRefs(); len(got) != 1 || got[0] != outRef {
		t.Fatalf("expected refs to forward through batch request")
	}
}

func TestBlockStorePutBlockBatchRejectsWrongExistenceLength(t *testing.T) {
	// Build a store whose remote answers with one result too few.
	client := &testBlockStoreClient{shortExisted: true}
	store := NewBlockStore(client, 0, false)

	// Send two entries and expect the mismatch instead of a short result.
	_, err := store.PutBlockBatch(context.Background(), []*block.PutBatchEntry{
		{Ref: &block.BlockRef{}, Data: []byte("one")},
		{Ref: &block.BlockRef{}, Data: []byte("two")},
	})
	if !errors.Is(err, ErrBatchExistenceMismatch) {
		t.Fatalf("err = %v, want %v", err, ErrBatchExistenceMismatch)
	}
}

// _ verifies the test client contract.
var _ block_rpc.SRPCBlockStoreClient = (*testBlockStoreClient)(nil)

func TestBlockStoreGetBlockSendsBackground(t *testing.T) {
	client := &testBlockStoreClient{}
	store := NewBlockStore(client, 0, false)
	for _, background := range []bool{true, false} {
		// Read through both block read paths with the class on the context.
		ctx := context.Background()
		if background {
			ctx = block.WithBackground(ctx)
		}
		if _, _, err := store.GetBlock(ctx, &block.BlockRef{}); err != nil {
			t.Fatal(err)
		}
		gotGet := client.getRequest.GetBackground()
		if _, err := store.GetStoredBlock(ctx, &block.BlockRef{}); err != nil {
			t.Fatal(err)
		}

		// Verify each request carries the class.
		if gotGet != background || client.getRequest.GetBackground() != background {
			t.Fatalf("background %v sent as GetBlock %v, GetStoredBlock %v", background, gotGet, client.getRequest.GetBackground())
		}
	}
}

func TestBlockStoreGetBlockRejectsWrongBytes(t *testing.T) {
	// Answer a read with bytes that do not hash to the requested ref.
	ref, err := block.BuildBlockRef([]byte("block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &testBlockStoreClient{getResponse: &block_rpc.GetBlockResponse{Data: []byte("other"), Exists: true}}
	store := NewBlockStore(client, 0, false)

	// Both read paths refuse the bytes as corrupt.
	if _, _, err := store.GetBlock(t.Context(), ref); !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("GetBlock of wrong bytes: %v, want ErrBlockCorrupt", err)
	}
	if _, err := store.GetStoredBlock(t.Context(), ref); !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("GetStoredBlock of wrong bytes: %v, want ErrBlockCorrupt", err)
	}
}

// corruptStore reads every block as corrupt, as a store whose disk returned
// the wrong bytes does.
type corruptStore struct {
	block.NopStoreOps
}

func (corruptStore) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return nil, false, &block.CorruptError{Ref: ref, StoreID: "disk"}
}

func (s corruptStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, s, ref)
}

// TestBlockStoreGetBlockKeepsCorruptClass checks that a corrupt read on the
// serving side reaches the client as ErrBlockCorrupt on both read paths, so
// the layers above a remote store take their corrupt branch.
func TestBlockStoreGetBlockKeepsCorruptClass(t *testing.T) {
	ref, err := block.BuildBlockRef([]byte("block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	server := block_rpc_server.NewBlockStore(corruptStore{})
	for _, withRefs := range []bool{false, true} {
		resp, err := server.GetBlock(t.Context(), &block_rpc.GetBlockRequest{Ref: ref, WithRefs: withRefs})
		if err != nil {
			t.Fatal(err)
		}
		store := NewBlockStore(&testBlockStoreClient{getResponse: resp}, 0, false)
		if withRefs {
			_, err = store.GetStoredBlock(t.Context(), ref)
		} else {
			_, _, err = store.GetBlock(t.Context(), ref)
		}
		if !errors.Is(err, block.ErrBlockCorrupt) {
			t.Fatalf("read with refs %v: %v, want ErrBlockCorrupt", withRefs, err)
		}
	}
}
