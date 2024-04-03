package block_store_inmem

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
)

// RistrettoBlock is a block store on top of a Ristretto cache.
// Stores blocks at {objectPrefix}/{block ref}
type RistrettoBlock = block_store_kvtx.KVTxBlock

// NewRistrettoBlock builds a new block store on top of a Ristretto cache.
//
// forceHashType can be 0 to use the default hash type.
// hashGet hashes Get requests for integrity, use if the storage is unreliable or untrusted.
func NewRistrettoBlock(
	kvk *kvkey.KVKey,
	st *store_kvtx_inmem.Store,
	forceHashType hash.HashType,
	hashGet bool,
) *RistrettoBlock {
	return block_store_kvtx.NewKVTxBlock(kvk, st, forceHashType, hashGet)
}

// _ is a type assertion
var _ block.StoreOps = ((*RistrettoBlock)(nil))
