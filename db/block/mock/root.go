package block_mock

import (
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// NewRootBlock constructs a new root block.
func NewRootBlock() block.Block {
	return &Root{}
}

// IsNil returns if the object is nil.
func (r *Root) IsNil() bool {
	return r == nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *Root) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *Root) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *Root) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 1:
		r.ExampleSubBlock, ok = next.(*SubBlock)
		if !ok {
			return errors.New("sub-block must be of type SubBlock")
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *Root) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	if sblock := r.GetExampleSubBlock(); sblock != nil {
		m[1] = sblock
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *Root) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(write bool) block.SubBlock {
			if ex := r.GetExampleSubBlock(); ex != nil || !write {
				return ex
			}
			r.ExampleSubBlock = &SubBlock{}
			return r.ExampleSubBlock
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Root)(nil))
	_ block.BlockWithSubBlocks = ((*Root)(nil))
)
