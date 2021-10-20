package world_mock

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/golang/protobuf/proto"
)

// MockWorldOpId is the mock object operation identifier.
var MockWorldOpId = "hydra/world/mock/mock-world-op"

// NewMockWorldOp constructs a new MockWorldOp block.
func NewMockWorldOp(objectKey, msg string) *MockWorldOp {
	return &MockWorldOp{
		ObjectKey: objectKey,
		NextMsg:   msg,
	}
}

// NewMockWorldOpBlock constructs a new MockWorldOp block.
func NewMockWorldOpBlock() block.Block {
	return &MockWorldOp{}
}

// LookupMockWorldOp performs the lookup operation for the mock object op.
func LookupMockWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID != MockWorldOpId {
		return nil, nil
	}
	return &MockWorldOp{}, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupMockWorldOp

// GetOperationTypeId returns the operation type identifier.
func (m *MockWorldOp) GetOperationTypeId() string {
	return MockWorldOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (m *MockWorldOp) ApplyWorldOp(
	ctx context.Context,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	nextMsg := m.GetNextMsg()
	objectKey := m.GetObjectKey()

	// re-use the logic for the object op
	objState, err := world.MustGetObject(worldHandle, objectKey)
	if err != nil {
		return false, err
	}

	op := NewMockObjectOp(nextMsg)
	return op.ApplyWorldObjectOp(ctx, objState, sender)
}

// ApplyWorldObjectOp applies the operation as a object operation.
func (m *MockWorldOp) ApplyWorldObjectOp(
	ctx context.Context,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
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
var (
	_ block.Block     = ((*MockWorldOp)(nil))
	_ world.Operation = ((*MockWorldOp)(nil))
)
