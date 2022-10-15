package block_rpc_server

import (
	"context"

	block_rpc "github.com/aperturerobotics/hydra/block/rpc"
	block_store "github.com/aperturerobotics/hydra/block/store"
)

// BlockStore implements the BlockStore RPC service.
type BlockStore struct {
	// store is the underlying block store
	store block_store.Store
}

// NewBlockStore constructs a new BlockStore from a Store.
func NewBlockStore(store block_store.Store) *BlockStore {
	return &BlockStore{
		store: store,
	}
}

// PutBlock stores a block into the store.
func (s *BlockStore) PutBlock(
	ctx context.Context,
	req *block_rpc.PutBlockRequest,
) (*block_rpc.PutBlockResponse, error) {
	outRef, existed, err := s.store.PutBlock(req.GetData(), req.GetPutOpts())
	resp := &block_rpc.PutBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Ref = outRef
		resp.Existed = existed
	}
	return resp, nil
}

// GetBlock returns a block from the store.
func (s *BlockStore) GetBlock(
	ctx context.Context,
	req *block_rpc.GetBlockRequest,
) (*block_rpc.GetBlockResponse, error) {
	data, existed, err := s.store.GetBlock(req.GetRef())
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
	existed, err := s.store.GetBlockExists(req.GetRef())
	resp := &block_rpc.GetBlockExistsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Exists = existed
	}
	return resp, nil
}

// RmBlock removes the block from the store.
func (s *BlockStore) RmBlock(
	ctx context.Context,
	req *block_rpc.RmBlockRequest,
) (*block_rpc.RmBlockResponse, error) {
	err := s.store.RmBlock(req.GetRef())
	resp := &block_rpc.RmBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// _ is a type assertion
var _ block_rpc.SRPCBlockStoreServer = ((*BlockStore)(nil))
