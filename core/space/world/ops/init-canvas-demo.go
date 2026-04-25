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

// InitCanvasDemo initializes a Canvas with demo content in a world.
// Returns any error.
func InitCanvasDemo(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewInitCanvasDemoOp(objKey, ts)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// InitCanvasDemoOpId is the operation id for InitCanvasDemoOp.
var InitCanvasDemoOpId = "space/world/init-canvas-demo"

// NewInitCanvasDemoOp constructs a new InitCanvasDemoOp block.
func NewInitCanvasDemoOp(
	objKey string,
	ts time.Time,
) *InitCanvasDemoOp {
	return &InitCanvasDemoOp{
		ObjectKey: objKey,
		Timestamp: timestamp.New(ts),
	}
}

// NewInitCanvasDemoOpBlock constructs a new InitCanvasDemoOp block.
func NewInitCanvasDemoOpBlock() block.Block {
	return &InitCanvasDemoOp{}
}

// Validate performs cursory checks on the op.
func (o *InitCanvasDemoOp) Validate() error {
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
func (o *InitCanvasDemoOp) GetOperationTypeId() string {
	return InitCanvasDemoOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitCanvasDemoOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create the canvas with a UnixFS demo node.
	unixfsNodeID := "unixfs-demo"
	state := &s4wave_canvas.CanvasState{
		Nodes: map[string]*s4wave_canvas.CanvasNode{
			unixfsNodeID: {
				Id:        unixfsNodeID,
				X:         100,
				Y:         100,
				Width:     400,
				Height:    300,
				Type:      s4wave_canvas.NodeType_NODE_TYPE_WORLD_OBJECT,
				ObjectKey: "files",
				Pinned:    true,
			},
		},
	}
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
func (o *InitCanvasDemoOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *InitCanvasDemoOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *InitCanvasDemoOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupInitCanvasDemoOp looks up a InitCanvasDemoOp operation type.
func LookupInitCanvasDemoOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == InitCanvasDemoOpId {
		return &InitCanvasDemoOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*InitCanvasDemoOp)(nil))
