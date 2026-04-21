package block_store_kvfile

import (
	"context"

	"github.com/s4wave/spacewave/net/hash"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_vlogger "github.com/s4wave/spacewave/db/block/store/vlogger"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
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
		blockStore := block_store.NewStore(blockStoreID, kvfileBlock)
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

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (k *KvfileBlock) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	key := k.kvkey.GetBlockKey(rm)

	size, err := k.store.GetValueSize(key)
	if err != nil || size < 0 {
		return nil, nil
	}

	return &block.BlockStat{Ref: ref, Size: size}, nil
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (k *KvfileBlock) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// _ is a type assertion
var _ block.StoreOps = ((*KvfileBlock)(nil))
