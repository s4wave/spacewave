package bstore

import (
	"context"
	"time"

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
	// ReclaimStorage drops the blocks no live root reaches from its storage
	// backend. fence runs after the backend lists its blocks and before it
	// drops any; when fence returns, every writer that may still reference a
	// dropped block must upload it again. A store without a storage backend,
	// or whose backend judges the pass would cost more than the storage it
	// frees, returns without calling fence.
	//
	// Returns the time a pass next comes due if the store is not written
	// again, or zero when none will. Asking again at that time runs the last
	// pass an idle store needs.
	ReclaimStorage(ctx context.Context, fence ReclaimFence) (time.Time, error)
}

// ReclaimFence places every operation a writer started before the fence and
// returns the live roots: every block a writer may still reference is under
// one of them. With copyRoots set, it also copies the graph of each root into
// the local store and holds it there until the pass ends, for a store that
// judges liveness by local existence.
type ReclaimFence func(ctx context.Context, copyRoots bool) ([]*block.BlockRef, error)

// Validate validates the block store ref.
func (r *BlockStoreRef) Validate() error {
	return r.GetProviderResourceRef().Validate()
}

// GetLogger adds debug values to the logger.
func (r *BlockStoreRef) GetLogger(le *logrus.Entry) *logrus.Entry {
	return r.GetProviderResourceRef().GetLogger(le)
}
