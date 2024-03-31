package block_mock

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
)

// NewMockStore constructs a new mock bucket for testing.
//
// hashType is the hash type to use, 0 for default.
func NewMockStore(hashType hash.HashType) block.Store {
	return block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hashType,
		false,
	)
}
