package kvtx_block

import (
	"github.com/aperturerobotics/hydra/block"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
)

// NewKeyValueStoreBlock constructs a new KeyValueStore block.
func NewKeyValueStoreBlock() block.Block {
	return &KeyValueStore{}
}

// NewKeyValueStoreSubBlockCtor returns the sub-block constructor.
func NewKeyValueStoreSubBlockCtor(r **KeyValueStore) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &KeyValueStore{ImplType: DefaultKeyValueStoreImpl}
			*r = v
		}
		return v
	}
}

// IsNil checks if the object is nil.
func (k *KeyValueStore) IsNil() bool {
	return k == nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (k *KeyValueStore) MarshalBlock() ([]byte, error) {
	return k.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (k *KeyValueStore) UnmarshalBlock(data []byte) error {
	return k.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (k *KeyValueStore) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		v, ok := next.(*iavl.Node)
		if !ok {
			return block.ErrUnexpectedType
		}
		k.IavlRoot = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (k *KeyValueStore) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	switch k.GetImplType() {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		m[2] = k.GetIavlRoot()
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (k *KeyValueStore) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return func(create bool) block.SubBlock {
			x := k.IavlRoot
			if x == nil && create {
				x = &iavl.Node{}
				k.IavlRoot = x
			}
			if x == nil {
				return nil
			}
			return x
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*KeyValueStore)(nil))
	_ block.BlockWithSubBlocks = ((*KeyValueStore)(nil))
)
