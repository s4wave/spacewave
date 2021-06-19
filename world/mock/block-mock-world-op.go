package world_mock

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/golang/protobuf/proto"
)

// MockWorldOpId is the mock world operation identifier.
var MockWorldOpId = "hydra/world/mock/mock-world-op"

// ApplyMockWorldOp applies a mock operation.
func ApplyMockWorldOp(
	ctx context.Context,
	worldHandle world.WorldState,
	operationTypeID string,
	op world.Operation,
) (handled bool, err error) {
	if operationTypeID != MockWorldOpId {
		return false, nil
	}

	return false, errors.New("TODO apply mock world op")
}

// NewMockWorldOp constructs a new MockWorldOp block.
func NewMockWorldOp(objectID, msg string) *MockWorldOp {
	return &MockWorldOp{ObjectId: objectID, NextMsg: msg}
}

// NewMockWorldOpBlock constructs a new MockWorldOp block.
func NewMockWorldOpBlock() block.Block {
	return &MockWorldOp{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (m *MockWorldOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(m)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (m *MockWorldOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, m)
}

// _ is a type assertion
var _ block.Block = ((*MockWorldOp)(nil))
