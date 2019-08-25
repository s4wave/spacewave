package block_mock

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
)

// NewExampleBlock builds a new example block.
func NewExampleBlock() block.Block {
	return &Example{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Example) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Example) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// ApplyBlockRef applies a ref change with a field id.
func (e *Example) ApplyBlockRef(id uint32, ptr *cid.BlockRef) error {
	return nil
}

// GetBlockRefs returns all filled block references.
// Note: this does not include pending references (in a cursor)
func (e *Example) GetBlockRefs() (map[uint32]*cid.BlockRef, error) {
	return nil, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (e *Example) GetBlockRefCtor(id uint32) block.Ctor {
	return nil
}

// _ is a type assertion
var _ block.Block = ((*Example)(nil))
