package block_mock

import (
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// NewMockStore constructs a new mock bucket for testing.
//
// hashType is the hash type to use, 0 for default.
func NewMockStore(hashType hash.HashType) block.StoreOps {
	return block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hashType,
		false,
	)
}
