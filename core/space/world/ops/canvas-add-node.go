package space_world_ops

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/sirupsen/logrus"
)

// CanvasAddNodeOpId is the operation id for CanvasAddNodeOp.
var CanvasAddNodeOpId = "space/world/canvas-add-node"

// NewCanvasAddNodeOp constructs a new CanvasAddNodeOp block.
func NewCanvasAddNodeOp(objKey string, node *s4wave_canvas.CanvasNode) *CanvasAddNodeOp {
	return &CanvasAddNodeOp{
		ObjectKey: objKey,
		Node:      node,
	}
}

// NewCanvasAddNodeOpBlock constructs a new CanvasAddNodeOp block.
func NewCanvasAddNodeOpBlock() block.Block {
	return &CanvasAddNodeOp{}
}

// Validate performs cursory checks on the op.
func (o *CanvasAddNodeOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	node := o.GetNode()
	if node == nil {
		return ErrNodeRequired
	}
	if len(node.GetId()) == 0 {
		return ErrNodeIdRequired
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *CanvasAddNodeOp) GetOperationTypeId() string {
	return CanvasAddNodeOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasAddNodeOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	node := o.GetNode()

	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		state, uerr := s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		if uerr != nil {
			return uerr
		}
		if state == nil {
			state = &s4wave_canvas.CanvasState{}
		}
		if state.Nodes == nil {
			state.Nodes = make(map[string]*s4wave_canvas.CanvasNode)
		}
		state.Nodes[node.GetId()] = node
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CanvasAddNodeOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CanvasAddNodeOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CanvasAddNodeOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCanvasAddNodeOp looks up a CanvasAddNodeOp operation type.
func LookupCanvasAddNodeOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CanvasAddNodeOpId {
		return &CanvasAddNodeOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CanvasAddNodeOp)(nil))
