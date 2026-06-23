package block_rpc_server

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

// BlockStore implements the BlockStore RPC service.
type BlockStore struct {
	// store is the underlying block store
	store block.StoreOps
}

// NewBlockStore constructs a new BlockStore from a Store.
func NewBlockStore(store block.StoreOps) *BlockStore {
	return &BlockStore{
		store: store,
	}
}

// GetHashType returns the preferred hash type for the store.
func (s *BlockStore) GetHashType(
	_ context.Context,
	_ *block_rpc.GetHashTypeRequest,
) (*block_rpc.GetHashTypeResponse, error) {
	return &block_rpc.GetHashTypeResponse{HashType: s.store.GetHashType()}, nil
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *BlockStore) GetSupportedFeatures(
	context.Context,
	*block_rpc.GetSupportedFeaturesRequest,
) (*block_rpc.GetSupportedFeaturesResponse, error) {
	return &block_rpc.GetSupportedFeaturesResponse{Features: s.store.GetSupportedFeatures()}, nil
}

// PutBlock stores a block into the store.
func (s *BlockStore) PutBlock(
	ctx context.Context,
	req *block_rpc.PutBlockRequest,
) (*block_rpc.PutBlockResponse, error) {
	outRef, existed, err := s.store.PutBlock(ctx, req.GetData(), req.GetPutOpts())
	resp := &block_rpc.PutBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Ref = outRef
		resp.Existed = existed
	}
	return resp, nil
}

// PutBlockBatch stores blocks into the store as a batch.
func (s *BlockStore) PutBlockBatch(
	ctx context.Context,
	req *block_rpc.PutBlockBatchRequest,
) (*block_rpc.PutBlockBatchResponse, error) {
	entries := make([]*block.PutBatchEntry, 0, len(req.GetEntries()))
	for _, entry := range req.GetEntries() {
		entries = append(entries, &block.PutBatchEntry{
			Ref:       entry.GetRef(),
			Data:      entry.GetData(),
			Refs:      entry.GetRefs(),
			Tombstone: entry.GetTombstone(),
		})
	}

	resp := &block_rpc.PutBlockBatchResponse{}
	if err := s.store.PutBlockBatch(ctx, entries); err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// GetBlock returns a block from the store.
func (s *BlockStore) GetBlock(
	ctx context.Context,
	req *block_rpc.GetBlockRequest,
) (*block_rpc.GetBlockResponse, error) {
	data, existed, err := s.store.GetBlock(ctx, req.GetRef())
	resp := &block_rpc.GetBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Data = data
		resp.Exists = existed
	}
	return resp, nil
}

// GetBlockExists checks if the block exists in the store.
func (s *BlockStore) GetBlockExists(
	ctx context.Context,
	req *block_rpc.GetBlockExistsRequest,
) (*block_rpc.GetBlockExistsResponse, error) {
	existed, err := s.store.GetBlockExists(ctx, req.GetRef())
	resp := &block_rpc.GetBlockExistsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Exists = existed
	}
	return resp, nil
}

// GetBlockExistsBatch checks if blocks exist in the store.
func (s *BlockStore) GetBlockExistsBatch(
	ctx context.Context,
	req *block_rpc.GetBlockExistsBatchRequest,
) (*block_rpc.GetBlockExistsBatchResponse, error) {
	resp := &block_rpc.GetBlockExistsBatchResponse{}
	exists, err := s.store.GetBlockExistsBatch(ctx, req.GetRefs())
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Exists = exists
	}
	return resp, nil
}

// RmBlock removes the block from the store.
func (s *BlockStore) RmBlock(
	ctx context.Context,
	req *block_rpc.RmBlockRequest,
) (*block_rpc.RmBlockResponse, error) {
	err := s.store.RmBlock(ctx, req.GetRef())
	resp := &block_rpc.RmBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// StatBlock returns metadata about a block without reading its data.
func (s *BlockStore) StatBlock(
	ctx context.Context,
	req *block_rpc.StatBlockRequest,
) (*block_rpc.StatBlockResponse, error) {
	stat, err := s.store.StatBlock(ctx, req.GetRef())
	resp := &block_rpc.StatBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	if stat == nil {
		return resp, nil
	}
	resp.Ref = stat.Ref
	resp.Size = stat.Size
	resp.Exists = true
	return resp, nil
}

// Sync drains buffered writes and blocks until prior writes are durable.
func (s *BlockStore) Sync(
	ctx context.Context,
	_ *block_rpc.SyncRequest,
) (*block_rpc.SyncResponse, error) {
	resp := &block_rpc.SyncResponse{}
	fenced, err := s.store.Sync(ctx)
	resp.Fenced = fenced
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// _ is a type assertion
var _ block_rpc.SRPCBlockStoreServer = ((*BlockStore)(nil))
