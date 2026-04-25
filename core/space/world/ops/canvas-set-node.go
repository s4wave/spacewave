package space_world_ops

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/sirupsen/logrus"
)

// CanvasSetNodeOpId is the operation id for CanvasSetNodeOp.
var CanvasSetNodeOpId = "space/world/canvas-set-node"

// NewCanvasSetNodeOp constructs a new CanvasSetNodeOp block.
func NewCanvasSetNodeOp(objKey string, node *s4wave_canvas.CanvasNode) *CanvasSetNodeOp {
	return &CanvasSetNodeOp{
		ObjectKey: objKey,
		Node:      node,
	}
}

// NewCanvasSetNodeOpBlock constructs a new CanvasSetNodeOp block.
func NewCanvasSetNodeOpBlock() block.Block {
	return &CanvasSetNodeOp{}
}

// Validate performs cursory checks on the op.
func (o *CanvasSetNodeOp) Validate() error {
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
func (o *CanvasSetNodeOp) GetOperationTypeId() string {
	return CanvasSetNodeOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasSetNodeOp) ApplyWorldOp(
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
	nodeID := node.GetId()

	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		state, uerr := s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		if uerr != nil {
			return uerr
		}
		if state == nil {
			return ErrNodeNotFound
		}
		if _, exists := state.Nodes[nodeID]; !exists {
			return ErrNodeNotFound
		}
		state.Nodes[nodeID] = node
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CanvasSetNodeOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CanvasSetNodeOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CanvasSetNodeOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCanvasSetNodeOp looks up a CanvasSetNodeOp operation type.
func LookupCanvasSetNodeOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CanvasSetNodeOpId {
		return &CanvasSetNodeOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CanvasSetNodeOp)(nil))
