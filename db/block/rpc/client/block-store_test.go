package block_rpc_client

import (
	"context"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

// testBlockStoreClient records feature and batch calls without a transport.
type testBlockStoreClient struct {
	features     block.StoreFeature
	featureCalls int
	batchEntries []*block_rpc.PutBlockBatchEntry
	batchHook    func(context.Context, *block_rpc.PutBlockBatchRequest) error
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
	c.batchEntries = req.GetEntries()
	if c.batchHook != nil {
		if err := c.batchHook(ctx, req); err != nil {
			return nil, err
		}
	}
	return &block_rpc.PutBlockBatchResponse{}, nil
}

func (c *testBlockStoreClient) GetBlock(
	context.Context,
	*block_rpc.GetBlockRequest,
) (*block_rpc.GetBlockResponse, error) {
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
	remote := block.StoreFeatureNativeBatchPut |
		block.StoreFeatureNativeBatchExists |
		block.StoreFeatureSelfBuffered
	client := &testBlockStoreClient{features: remote}
	store := NewBlockStore(client, 0, true)

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

	got := store.GetSupportedFeatures()
	if got != expected {
		t.Fatalf("expected cached read-only feature set, got %v", got)
	}
	if client.featureCalls != 1 {
		t.Fatalf("expected one feature RPC call, got %d", client.featureCalls)
	}
}

func TestBlockStorePutBlockBatchForwardsRefs(t *testing.T) {
	client := &testBlockStoreClient{}
	store := NewBlockStore(client, 0, false)
	ref := &block.BlockRef{}
	outRef := &block.BlockRef{}

	if err := store.PutBlockBatch(context.Background(), []*block.PutBatchEntry{{
		Ref:  ref,
		Data: []byte("hello"),
		Refs: []*block.BlockRef{outRef},
	}}); err != nil {
		t.Fatal(err.Error())
	}

	if len(client.batchEntries) != 1 {
		t.Fatalf("expected one batch entry, got %d", len(client.batchEntries))
	}
	if got := client.batchEntries[0].GetRefs(); len(got) != 1 || got[0] != outRef {
		t.Fatalf("expected refs to forward through batch request")
	}
}

// _ verifies the test client contract.
var _ block_rpc.SRPCBlockStoreClient = (*testBlockStoreClient)(nil)
