package rpc_block_server

import (
	"context"

	rpc_block "github.com/aperturerobotics/bldr/rpc/block"
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
	req *rpc_block.PutBlockRequest,
) (*rpc_block.PutBlockResponse, error) {
	outRef, existed, err := s.store.PutBlock(req.GetData(), req.GetPutOpts())
	resp := &rpc_block.PutBlockResponse{}
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
	req *rpc_block.GetBlockRequest,
) (*rpc_block.GetBlockResponse, error) {
	data, existed, err := s.store.GetBlock(req.GetRef())
	resp := &rpc_block.GetBlockResponse{}
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
	req *rpc_block.GetBlockExistsRequest,
) (*rpc_block.GetBlockExistsResponse, error) {
	existed, err := s.store.GetBlockExists(req.GetRef())
	resp := &rpc_block.GetBlockExistsResponse{}
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
	req *rpc_block.RmBlockRequest,
) (*rpc_block.RmBlockResponse, error) {
	err := s.store.RmBlock(req.GetRef())
	resp := &rpc_block.RmBlockResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// _ is a type assertion
var _ rpc_block.SRPCBlockStoreServer = ((*BlockStore)(nil))
