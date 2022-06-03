package block_mock

import (
	"github.com/aperturerobotics/hydra/block"
	proto "google.golang.org/protobuf/proto"
)

// NewSubBlockBlock constructs a SubBlock as a Block.
func NewSubBlockBlock() block.Block {
	return &SubBlock{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *SubBlock) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *SubBlock) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyBlockRef applies a ref change with a field id.
func (r *SubBlock) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		r.ExamplePtr = ptr
	}
	return nil
}

// GetBlockRefs returns all filled block references.
func (r *SubBlock) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		1: r.GetExamplePtr(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (r *SubBlock) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		return NewExampleBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*SubBlock)(nil))
	_ block.BlockWithRefs = ((*SubBlock)(nil))
)
