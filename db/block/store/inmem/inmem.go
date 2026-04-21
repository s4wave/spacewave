package block_store_inmem

import (
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// InmemBlock is a block store on top of a Ristretto cache.
// Stores blocks at {objectPrefix}/{block ref}
type InmemBlock = block_store_kvtx.KVTxBlock

// NewInmemBlock builds a new block store on top of a Ristretto cache.
//
// forceHashType can be 0 to use the default hash type.
// hashGet hashes Get requests for integrity, use if the storage is unreliable or untrusted.
func NewInmemBlock(
	kvk *kvkey.KVKey,
	st *store_kvtx_inmem.Store,
	forceHashType hash.HashType,
	hashGet bool,
) *InmemBlock {
	return block_store_kvtx.NewKVTxBlock(kvk, st, forceHashType, hashGet)
}

// _ is a type assertion
var _ block.StoreOps = ((*InmemBlock)(nil))
