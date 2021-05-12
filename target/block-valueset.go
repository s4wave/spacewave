package forge_target

import (
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (v *ValueSet) MarshalBlock() ([]byte, error) {
	return proto.Marshal(v)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (v *ValueSet) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, v)
}

// ApplySubBlock applies a sub-block change with a field id.
func (v *ValueSet) ApplySubBlock(id uint32, next block.SubBlock) error {
	// ignore: sub-block set always points to field in v
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (v *ValueSet) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	if inp := v.GetInputs(); inp != nil {
		m[1] = forge_value.NewValueSetContainer(&v.Inputs, nil)
	}
	if out := v.GetOutputs(); out != nil {
		m[2] = forge_value.NewValueSetContainer(&v.Outputs, nil)
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (v *ValueSet) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return forge_value.NewValueSetContainer(&v.Inputs, nil)
		}
	case 2:
		return func(create bool) block.SubBlock {
			return forge_value.NewValueSetContainer(&v.Outputs, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*ValueSet)(nil))
	_ block.BlockWithSubBlocks = ((*ValueSet)(nil))
)
