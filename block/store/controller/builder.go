package block_store_controller

import (
	"context"

	block_store "github.com/aperturerobotics/hydra/block/store"
)

// BlockStoreBuilder builds a block store.
//
// returns the store and an optional release function
// can return nil to indicate not found.
type BlockStoreBuilder func(ctx context.Context, released func()) (*block_store.Store, func(), error)

// NewBlockStoreBuilder creates a new BlockStoreBuilder with a static block store.
func NewBlockStoreBuilder(store block_store.Store) BlockStoreBuilder {
	return func(ctx context.Context, released func()) (*block_store.Store, func(), error) {
		if store == nil {
			return nil, nil, nil
		}
		return &store, nil, nil
	}
}
