package block_kvtx

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/iavl"
	"github.com/golang/protobuf/proto"
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
			v = &KeyValueStore{}
			*r = v
		}
		return v
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (k *KeyValueStore) MarshalBlock() ([]byte, error) {
	return proto.Marshal(k)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (k *KeyValueStore) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, k)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (k *KeyValueStore) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		k.IavlRoot = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (k *KeyValueStore) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[2] = k.GetIavlRoot()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (k *KeyValueStore) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return iavl.NewNodeBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*KeyValueStore)(nil))
	_ block.BlockWithRefs = ((*KeyValueStore)(nil))
)
