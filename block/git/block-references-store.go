package git

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/iavl"
	"github.com/golang/protobuf/proto"
)

// NewReferencesStoreBlock builds a new repo references block.
func NewReferencesStoreBlock() block.Block {
	return &ReferencesStore{}
}

// BuildRefTree builds the iavl tree.
//
// Bcs should be located at r.
func (r *ReferencesStore) BuildRefTree(bcs *block.Cursor) (*iavl.Tx, error) {
	return iavl.BuildIavlSubBlockTree(1, bcs, r)
}

// MarshalBlock marshals the block to binary.
func (r *ReferencesStore) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *ReferencesStore) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *ReferencesStore) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*iavl.Node)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.IavlRoot = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *ReferencesStore) GetSubBlocks() map[uint32]block.SubBlock {
	if r == nil {
		return nil
	}

	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetIavlRoot()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *ReferencesStore) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	switch id {
	case 1:
		return iavl.NewAVLTreeSubBlockCtor(&r.IavlRoot)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = (*ReferencesStore)(nil)
	_ block.BlockWithSubBlocks = (*ReferencesStore)(nil)
)
