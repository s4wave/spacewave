package git

import (
	"github.com/aperturerobotics/hydra/block"
	block_kvtx "github.com/aperturerobotics/hydra/block/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/golang/protobuf/proto"
)

// NewModuleReferencesStoreBlock builds a new modules references block.
func NewModuleReferencesStoreBlock() block.Block {
	return &ModuleReferencesStore{}
}

// BuildModRefTree builds the iavl tree.
//
// Bcs should be located at r.
func (r *ModuleReferencesStore) BuildModRefTree(bcs *block.Cursor) (kvtx.BlockTx, error) {
	return block_kvtx.BuildKvTransaction(bcs.FollowSubBlock(1), true)
}

// MarshalBlock marshals the block to binary.
func (r *ModuleReferencesStore) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *ModuleReferencesStore) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *ModuleReferencesStore) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*block_kvtx.KeyValueStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.KvtxRoot = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *ModuleReferencesStore) GetSubBlocks() map[uint32]block.SubBlock {
	if r == nil {
		return nil
	}

	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetKvtxRoot()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *ModuleReferencesStore) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	switch id {
	case 1:
		return block_kvtx.NewKeyValueStoreSubBlockCtor(&r.KvtxRoot)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = (*ModuleReferencesStore)(nil)
	_ block.BlockWithSubBlocks = (*ModuleReferencesStore)(nil)
)
