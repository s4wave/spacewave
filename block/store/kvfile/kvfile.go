package block_store_kvfile

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_controller "github.com/aperturerobotics/hydra/block/store/controller"
	block_store_vlogger "github.com/aperturerobotics/hydra/block/store/vlogger"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/sirupsen/logrus"
)

// KvfileBlock is a read-only block store on top of a kvfile.
type KvfileBlock struct {
	ctx   context.Context
	kvkey *store_kvkey.KVKey
	store *kvfile.Reader
}

// NewKvfileBlock constructs a new block store on top of a kvtx store.
//
// hashType can be 0 to use a default value.
func NewKvfileBlock(ctx context.Context, kvkey *store_kvkey.KVKey, store *kvfile.Reader) *KvfileBlock {
	return &KvfileBlock{ctx: ctx, kvkey: kvkey, store: store}
}

// NewBlockStoreBuilder constructs a new block store builder from a file open callback.
//
// le can be nil to disable logging
func NewBlockStoreBuilder(
	le *logrus.Entry,
	blockStoreID string,
	kvkey *store_kvkey.KVKey,
	openFile func() (kvfile.FileReaderAt, error),
	verbose bool,
) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		fd, err := openFile()
		if err != nil {
			return nil, nil, err
		}

		rdr, err := kvfile.BuildReaderWithFile(fd)
		if err != nil {
			_ = fd.Close()
			return nil, nil, err
		}

		kvfileBlock := NewKvfileBlock(ctx, kvkey, rdr)
		var blockStore block_store.Store = block_store.NewStore(blockStoreID, kvfileBlock)
		if verbose {
			blockStore = block_store_vlogger.NewVLoggerStore(le, blockStore)
		}
		return blockStore, func() { _ = fd.Close() }, nil
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (k *KvfileBlock) GetHashType() hash.HashType {
	return 0
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KvfileBlock) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	return nil, false, block_store.ErrReadOnly
}

// GetBlock looks up a block in the store.
// Returns data, found, and any unexpected error.
func (k *KvfileBlock) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	return k.store.Get(key)
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any unexpected error.
func (k *KvfileBlock) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	return k.store.Exists(key)
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (k *KvfileBlock) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// _ is a type assertion
var _ block.StoreOps = ((*KvfileBlock)(nil))
