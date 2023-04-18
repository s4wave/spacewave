package assembly_block

import (
	"github.com/aperturerobotics/hydra/block"
)

// NewDirectiveBridgeBlock builds a new DirectiveBridge block.
func NewDirectiveBridgeBlock() block.Block {
	return &DirectiveBridge{}
}

// IsNil checks if the object is nil.
func (r *DirectiveBridge) IsNil() bool {
	return r == nil
}

// MarshalBlock marshals the block to binary.
func (r *DirectiveBridge) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *DirectiveBridge) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// _ is a type assertion
var (
	_ block.Block    = ((*DirectiveBridge)(nil))
	_ block.SubBlock = ((*DirectiveBridge)(nil))
)
