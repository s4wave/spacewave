package world_mock

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// NewMockObjectOp constructs a new MockObjectOp block.
func NewMockObjectOp(msg string) *MockObjectOp {
	return &MockObjectOp{NextMsg: msg}
}

// NewMockObjectOpBlock constructs a new MockObjectOp block.
func NewMockObjectOpBlock() block.Block {
	return &MockObjectOp{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (m *MockObjectOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(m)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (m *MockObjectOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, m)
}

// _ is a type assertion
var _ block.Block = ((*MockObjectOp)(nil))
