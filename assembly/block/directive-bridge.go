package assembly_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// NewDirectiveBridgeBlock builds a new DirectiveBridge block.
func NewDirectiveBridgeBlock() block.Block {
	return &DirectiveBridge{}
}

// MarshalBlock marshals the block to binary.
func (r *DirectiveBridge) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *DirectiveBridge) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// _ is a type assertion
var _ block.Block = ((*DirectiveBridge)(nil))
