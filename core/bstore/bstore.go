package bstore

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/sirupsen/logrus"
)

// BlockStore is the block store handle interface.
type BlockStore interface {
	// Store is the block store interface.
	block_store.Store
	// GetDecodedBlockCache returns the lifecycle-owned decoded-block cache.
	GetDecodedBlockCache() *block.DecodedBlockCache
	// ReclaimStorage drops the blocks the store no longer holds from its
	// storage backend. fence runs after the backend lists its blocks and
	// before it drops any; when fence returns, every writer that may still
	// reference a dropped block must upload it again. A store without a
	// storage backend, or whose backend judges the pass would cost more than
	// the storage it frees, returns nil without calling fence.
	ReclaimStorage(ctx context.Context, fence func(context.Context) error) error
}

// Validate validates the block store ref.
func (r *BlockStoreRef) Validate() error {
	return r.GetProviderResourceRef().Validate()
}

// GetLogger adds debug values to the logger.
func (r *BlockStoreRef) GetLogger(le *logrus.Entry) *logrus.Entry {
	return r.GetProviderResourceRef().GetLogger(le)
}
