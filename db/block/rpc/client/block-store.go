package block_rpc_client

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// BlockStore implements a BlockStore backed by a BlockStore service.
type BlockStore struct {
	// client is the client to use
	client block_rpc.SRPCBlockStoreClient
	// hashType is the preferred hash type to use for writes
	hashType hash.HashType
	// readOnly disables write calls
	readOnly bool
	// beginErr stores BeginDeferFlush errors until EndDeferFlush can report them.
	beginErr error
	// beginErrMtx guards beginErr.
	beginErrMtx sync.Mutex
	// featuresOnce guards the lazy lookup of supportedFeatures.
	featuresOnce sync.Once
	// supportedFeatures caches the remote feature bitmask after the first call.
	supportedFeatures block.StoreFeature
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

// GetSupportedFeatures returns the native feature bitmask for the remote store.
// The result is cached after the first call: the remote feature set is static
// for the lifetime of the store, and this method is on the hot path.
func (v *BlockStore) GetSupportedFeatures() block.StoreFeature {
	v.featuresOnce.Do(func() {
		resp, err := v.client.GetSupportedFeatures(context.Background(), &block_rpc.GetSupportedFeaturesRequest{})
		if err != nil {
			v.supportedFeatures = block.StoreFeature_STORE_FEATURE_UNKNOWN
			return
		}
		v.supportedFeatures = resp.GetFeatures()
	})
	return v.supportedFeatures
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
func (v *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if v.readOnly {
		return nil, false, block_store.ErrReadOnly
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
	if err := addedRef.Validate(false); err != nil {
		return nil, false, err
	}
	return addedRef, resp.GetExisted(), nil
}

// PutBlockBatch requests a remote batch write.
func (v *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if v.readOnly {
		return block_store.ErrReadOnly
	}
	req := &block_rpc.PutBlockBatchRequest{Entries: make([]*block_rpc.PutBlockBatchEntry, 0, len(entries))}
	for _, entry := range entries {
		req.Entries = append(req.Entries, &block_rpc.PutBlockBatchEntry{
			Ref:       entry.Ref,
			Data:      entry.Data,
			Refs:      entry.Refs,
			Tombstone: entry.Tombstone,
		})
	}
	resp, err := v.client.PutBlockBatch(ctx, req)
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// PutBlockBackground requests a remote background write.
func (v *BlockStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if v.readOnly {
		return nil, false, block_store.ErrReadOnly
	}
	resp, err := v.client.PutBlockBackground(ctx, &block_rpc.PutBlockBackgroundRequest{
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
	if err := addedRef.Validate(false); err != nil {
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

// GetBlockExistsBatch requests a remote batch existence check.
func (v *BlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	resp, err := v.client.GetBlockExistsBatch(ctx, &block_rpc.GetBlockExistsBatchRequest{Refs: refs})
	if err != nil {
		return nil, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, errors.New(errStr)
	}
	return resp.GetExists(), nil
}

// StatBlock returns metadata about a block without reading its data.
// Falls back to GetBlockExists and returns Size=-1 (unknown).
// Returns nil, nil if the block does not exist.
func (v *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	found, err := v.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: -1}, nil
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (v *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if v.readOnly {
		return block_store.ErrReadOnly
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

// Flush requests a remote flush.
func (v *BlockStore) Flush(ctx context.Context) error {
	resp, err := v.client.Flush(ctx, &block_rpc.FlushRequest{})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// BeginDeferFlush opens a remote defer-flush scope.
func (v *BlockStore) BeginDeferFlush() {
	resp, err := v.client.BeginDeferFlush(context.Background(), &block_rpc.BeginDeferFlushRequest{})
	if err == nil {
		if errStr := resp.GetError(); errStr != "" {
			err = errors.New(errStr)
		}
	}
	if err != nil {
		v.beginErrMtx.Lock()
		if v.beginErr == nil {
			v.beginErr = err
		}
		v.beginErrMtx.Unlock()
	}
}

// EndDeferFlush closes a remote defer-flush scope.
func (v *BlockStore) EndDeferFlush(ctx context.Context) error {
	v.beginErrMtx.Lock()
	beginErr := v.beginErr
	v.beginErr = nil
	v.beginErrMtx.Unlock()
	if beginErr != nil {
		return beginErr
	}
	resp, err := v.client.EndDeferFlush(ctx, &block_rpc.EndDeferFlushRequest{})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// _ is a type assertion
var _ block.StoreOps = ((*BlockStore)(nil))
