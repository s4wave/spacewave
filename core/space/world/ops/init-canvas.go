package space_world_ops

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	"github.com/sirupsen/logrus"
)

// InitCanvas initializes a blank Canvas in a world.
// Returns any error.
func InitCanvas(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewCanvasInitOp(objKey, ts)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// CanvasInitOpId is the operation id for CanvasInitOp.
var CanvasInitOpId = "space/world/init-canvas"

// NewCanvasInitOp constructs a new CanvasInitOp block.
func NewCanvasInitOp(
	objKey string,
	ts time.Time,
) *CanvasInitOp {
	return &CanvasInitOp{
		ObjectKey: objKey,
		Timestamp: timestamp.New(ts),
	}
}

// NewCanvasInitOpBlock constructs a new CanvasInitOp block.
func NewCanvasInitOpBlock() block.Block {
	return &CanvasInitOp{}
}

// Validate performs cursory checks on the op.
func (o *CanvasInitOp) Validate() error {
	objKey := o.GetObjectKey()
	if len(objKey) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *CanvasInitOp) GetOperationTypeId() string {
	return CanvasInitOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasInitOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create a blank canvas.
	state := &s4wave_canvas.CanvasState{}
	_, _, err = world.CreateWorldObject(ctx, worldHandle, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	// Set the canvas object type.
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, s4wave_canvas_world.CanvasTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CanvasInitOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CanvasInitOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CanvasInitOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCanvasInitOp looks up a CanvasInitOp operation type.
func LookupCanvasInitOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CanvasInitOpId {
		return &CanvasInitOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CanvasInitOp)(nil))
