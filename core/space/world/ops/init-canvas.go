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

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasInitOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the blank canvas initialization and select its object key.
	objKey := o.GetObjectKey()
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create a blank canvas.
	state := &s4wave_canvas.CanvasState{}
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, worldHandle, objKey, func(bcs *block.Cursor) error {
		return s4wave_canvas.WriteCanvasState(ctx, bcs, nil, state)
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Set the canvas object type.
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, s4wave_canvas_world.CanvasTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*CanvasInitOp)(nil)
