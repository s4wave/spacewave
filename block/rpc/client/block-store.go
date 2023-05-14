package block_rpc_client

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_rpc "github.com/aperturerobotics/hydra/block/rpc"
	block_store "github.com/aperturerobotics/hydra/block/store"
)

// BlockStore implements a BlockStore backed by a BlockStore service.
type BlockStore struct {
	// client is the client to use
	client block_rpc.SRPCBlockStoreClient
	// hashType is the preferred hash type to use for writes
	hashType hash.HashType
	// readOnly disables write calls
	readOnly bool
}

// NewBlockStore constructs a new BlockStore.
func NewBlockStore(
	client block_rpc.SRPCBlockStoreClient,
	hashType hash.HashType,
	readOnly bool,
) *BlockStore {
	return &BlockStore{
		client:   client,
		hashType: hashType,
		readOnly: readOnly,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (v *BlockStore) GetHashType() hash.HashType {
	return v.hashType
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
func (v *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if v.readOnly {
		return nil, false, block_store.ErrReadOnlyStore
	}
	resp, err := v.client.PutBlock(ctx, &block_rpc.PutBlockRequest{
		Data:    data,
		PutOpts: opts,
	})
	if err != nil {
		return nil, false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, false, errors.New(errStr)
	}
	addedRef := resp.GetRef()
	if err := addedRef.Validate(); err != nil {
		return nil, false, err
	}
	return addedRef, resp.GetExisted(), nil
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (v *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	resp, err := v.client.GetBlock(ctx, &block_rpc.GetBlockRequest{
		Ref: ref.Clone(),
	})
	if err != nil {
		return nil, false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, false, errors.New(errStr)
	}
	return resp.GetData(), resp.GetExists(), nil
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (v *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	resp, err := v.client.GetBlockExists(ctx, &block_rpc.GetBlockExistsRequest{
		Ref: ref.Clone(),
	})
	if err != nil {
		return false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return false, errors.New(errStr)
	}
	return resp.GetExists(), nil
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (v *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if v.readOnly {
		return block_store.ErrReadOnlyStore
	}
	resp, err := v.client.RmBlock(ctx, &block_rpc.RmBlockRequest{
		Ref: ref.Clone(),
	})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// _ is a type assertion
var _ block_store.Store = ((*BlockStore)(nil))
