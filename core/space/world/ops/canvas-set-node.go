package space_world_ops

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/sirupsen/logrus"
)

// NewCanvasSetNodeOp constructs a new CanvasSetNodeOp block.
func NewCanvasSetNodeOp(objKey string, node *s4wave_canvas.CanvasNode) *CanvasSetNodeOp {
	return &CanvasSetNodeOp{
		ObjectKey: objKey,
		Node:      node,
	}
}

// Validate performs cursory checks on the op.
func (o *CanvasSetNodeOp) Validate() error {
	// Require a target object and a canvas node with an ID.
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

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasSetNodeOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the canvas node replacement before accessing its object.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Select the canvas object and node ID to replace.
	objKey := o.GetObjectKey()
	node := o.GetNode()
	nodeID := node.GetId()

	// Update the existing node in the canvas object.
	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		// Load the canvas state and require the target node to exist.
		state, uerr := s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		if uerr != nil {
			return uerr
		}
		if state == nil {
			return world.NewOpRefusedError(ErrNodeNotFound)
		}
		if _, exists := state.Nodes[nodeID]; !exists {
			return world.NewOpRefusedError(ErrNodeNotFound)
		}

		// Preserve the previous canvas state and persist the replacement node.
		previous := state.CloneVT()
		state.Nodes[nodeID] = node
		return s4wave_canvas.WriteCanvasState(ctx, bcs, previous, state)
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*CanvasSetNodeOp)(nil)
