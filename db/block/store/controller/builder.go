package block_store_controller

import (
	"context"

	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_vlogger "github.com/s4wave/spacewave/db/block/store/vlogger"
	"github.com/aperturerobotics/util/refcount"
	"github.com/sirupsen/logrus"
)

// BlockStoreBuilder builds a block store.
//
// returns the store and an optional release function
// can return nil to indicate not found.
type BlockStoreBuilder = refcount.RefCountResolver[block_store.Store]

// NewBlockStoreBuilder creates a new BlockStoreBuilder with a static block store.
func NewBlockStoreBuilder(store block_store.Store) BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		if store == nil {
			return nil, nil, nil
		}
		return store, nil, nil
	}
}

// WrapVerboseBlockStoreBuilder wraps a BlockStoreBuilder to be verbose.
func WrapVerboseBlockStoreBuilder(le *logrus.Entry, builder BlockStoreBuilder) BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		st, rel, err := builder(ctx, released)
		if err == nil && st != nil {
			st = block_store_vlogger.NewVLoggerStore(le, st)
		}
		return st, rel, err
	}
}
