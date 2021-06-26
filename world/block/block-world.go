package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	block_kvtx "github.com/aperturerobotics/hydra/kvtx/block"
	"github.com/golang/protobuf/proto"
)

// NewWorldBlock constructs a new world state block.
func NewWorldBlock() block.Block {
	return &World{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *World) MarshalBlock() ([]byte, error) {
	return proto.Marshal(w)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *World) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, w)
}

// ApplySubBlock applies a sub-block change with a field id.
func (w *World) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*block_kvtx.KeyValueStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.ObjectKeyValue = v
	case 2:
		v, ok := next.(*block_kvtx.KeyValueStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.GraphKeyValue = v
	case 3:
		v, ok := next.(*WorldChange)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.LastChange = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (w *World) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = w.GetObjectKeyValue()
	m[2] = w.GetGraphKeyValue()
	m[3] = w.GetLastChange()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (w *World) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return block_kvtx.NewKeyValueStoreSubBlockCtor(&w.ObjectKeyValue)
	case 2:
		return block_kvtx.NewKeyValueStoreSubBlockCtor(&w.GraphKeyValue)
	case 3:
		return NewWorldChangeSubBlockCtor(&w.LastChange)
	default:
		return nil
	}
}

// _ is a type assertion
var (
	_ block.Block              = ((*World)(nil))
	_ block.BlockWithSubBlocks = ((*World)(nil))
)
