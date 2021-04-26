package forge_target

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// Validate performs cursory validation of the target.
func (t *Target) Validate() error {
	// ensure all input names are unique
	// ensure all output names are unique
	// TODO
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *Target) MarshalBlock() ([]byte, error) {
	return proto.Marshal(t)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *Target) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, t)
}

// ApplySubBlock applies a sub-block change with a field id.
func (t *Target) ApplySubBlock(id uint32, next block.SubBlock) error {
	// noop
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (t *Target) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = t.GetSubBlockCtor(1)(false)
	m[2] = t.GetSubBlockCtor(2)(false)
	m[3] = t.GetSubBlockCtor(3)(false)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *Target) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(bool) block.SubBlock {
			return newInputSetContainer(&t.Inputs, nil)
		}
	case 2:
		return func(bool) block.SubBlock {
			return newOutputSetContainer(&t.Outputs, nil)
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := t.Exec
			if v == nil && create {
				v = &Exec{}
				t.Exec = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Target)(nil))
	_ block.BlockWithSubBlocks = ((*Target)(nil))
)
