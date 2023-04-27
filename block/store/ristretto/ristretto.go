package block_store_ristretto

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_ristretto "github.com/aperturerobotics/hydra/store/kvtx/ristretto"
)

// RistrettoBlock is a block store on top of a Ristretto cache.
// Stores blocks at {objectPrefix}/{block ref}
type RistrettoBlock = block_store_kvtx.KVTxBlock

// NewRistrettoBlock builds a new block store on top of a Ristretto cache.
//
// forceHashType can be 0 to use the default hash type.
func NewRistrettoBlock(
	ctx context.Context,
	kvk *kvkey.KVKey,
	st *store_kvtx_ristretto.Store,
	forceHashType hash.HashType,
) *RistrettoBlock {
	return block_store_kvtx.NewKVTxBlock(ctx, kvk, st, forceHashType)
}

// _ is a type assertion
var _ block_store.Store = ((*RistrettoBlock)(nil))
