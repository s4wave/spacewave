package world_mock

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewMockObjectOp constructs a new MockObjectOp block.
func NewMockObjectOp(msg string) *MockObjectOp {
	return &MockObjectOp{NextMsg: msg}
}

// Validate performs cursory checks on the op.
func (m *MockObjectOp) Validate() error {
	if len(m.GetNextMsg()) == 0 {
		return ErrEmptyNextMsg
	}
	return nil
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
		// Read or create the example block and replace its message in the object cursor.
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

	// A failure here is the store's, not a verdict on the operation.
	return err != nil, err
}

// _ is a type assertion
var _ block.Block = (*MockObjectOp)(nil)
