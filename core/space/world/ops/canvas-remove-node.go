package space_world_ops

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/sirupsen/logrus"
)

// CanvasRemoveNodeOpId is the operation id for CanvasRemoveNodeOp.
var CanvasRemoveNodeOpId = "space/world/canvas-remove-node"

// NewCanvasRemoveNodeOp constructs a new CanvasRemoveNodeOp block.
func NewCanvasRemoveNodeOp(objKey string, nodeIDs []string) *CanvasRemoveNodeOp {
	return &CanvasRemoveNodeOp{
		ObjectKey: objKey,
		NodeIds:   nodeIDs,
	}
}

// NewCanvasRemoveNodeOpBlock constructs a new CanvasRemoveNodeOp block.
func NewCanvasRemoveNodeOpBlock() block.Block {
	return &CanvasRemoveNodeOp{}
}

// Validate performs cursory checks on the op.
func (o *CanvasRemoveNodeOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(o.GetNodeIds()) == 0 {
		return ErrNodeIdsRequired
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *CanvasRemoveNodeOp) GetOperationTypeId() string {
	return CanvasRemoveNodeOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasRemoveNodeOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	nodeIDs := o.GetNodeIds()

	// Build a set of node IDs to remove.
	removedSet := make(map[string]struct{}, len(nodeIDs))
	for _, id := range nodeIDs {
		removedSet[id] = struct{}{}
	}

	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		state, uerr := s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		if uerr != nil {
			return uerr
		}
		if state == nil {
			state = &s4wave_canvas.CanvasState{}
		}

		// Remove nodes.
		for id := range removedSet {
			delete(state.Nodes, id)
		}

		// Remove edges referencing deleted nodes.
		for i, edge := range slices.Backward(state.Edges) {
			if _, removed := removedSet[edge.GetSourceNodeId()]; removed {
				state.Edges = append(state.Edges[:i], state.Edges[i+1:]...)
				continue
			}
			if _, removed := removedSet[edge.GetTargetNodeId()]; removed {
				state.Edges = append(state.Edges[:i], state.Edges[i+1:]...)
			}
		}

		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CanvasRemoveNodeOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CanvasRemoveNodeOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CanvasRemoveNodeOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCanvasRemoveNodeOp looks up a CanvasRemoveNodeOp operation type.
func LookupCanvasRemoveNodeOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CanvasRemoveNodeOpId {
		return &CanvasRemoveNodeOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CanvasRemoveNodeOp)(nil))
