package world_mock

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// MockObjectOpId is the mock object operation identifier.
var MockObjectOpId = "hydra/world/mock/mock-object-op"

// NewMockObjectOp constructs a new MockObjectOp block.
func NewMockObjectOp(msg string) *MockObjectOp {
	return &MockObjectOp{NextMsg: msg}
}

// NewMockObjectOpBlock constructs a new MockObjectOp block.
func NewMockObjectOpBlock() block.Block {
	return &MockObjectOp{}
}

// LookupMockObjectOp performs the lookup operation for the mock object op.
func LookupMockObjectOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID != MockObjectOpId {
		return nil, nil
	}
	return &MockObjectOp{}, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupMockObjectOp

// Validate performs cursory checks on the op.
func (m *MockObjectOp) Validate() error {
	if len(m.GetNextMsg()) == 0 {
		return ErrEmptyNextMsg
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (m *MockObjectOp) GetOperationTypeId() string {
	return MockObjectOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (m *MockObjectOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// ApplyWorldObjectOp applies the operation as a object operation.
func (m *MockObjectOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	nextMsg := m.GetNextMsg()

	// update and/or create the object.
	// if there was no change, this will have no effect.
	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		ex, err := block_mock.UnmarshalExample(ctx, bcs)
		if err != nil {
			return err
		}
		if ex == nil {
			ex = &block_mock.Example{}
		}
		ex.Msg = nextMsg
		bcs.SetBlock(ex, true)
		return nil
	})

	return true, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (m *MockObjectOp) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (m *MockObjectOp) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*MockObjectOp)(nil))
