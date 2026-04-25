package space_world_ops

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/sirupsen/logrus"
)

// CanvasAddEdgeOpId is the operation id for CanvasAddEdgeOp.
var CanvasAddEdgeOpId = "space/world/canvas-add-edge"

// NewCanvasAddEdgeOp constructs a new CanvasAddEdgeOp block.
func NewCanvasAddEdgeOp(objKey string, edge *s4wave_canvas.CanvasEdge) *CanvasAddEdgeOp {
	return &CanvasAddEdgeOp{
		ObjectKey: objKey,
		Edge:      edge,
	}
}

// NewCanvasAddEdgeOpBlock constructs a new CanvasAddEdgeOp block.
func NewCanvasAddEdgeOpBlock() block.Block {
	return &CanvasAddEdgeOp{}
}

// Validate performs cursory checks on the op.
func (o *CanvasAddEdgeOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	edge := o.GetEdge()
	if edge == nil {
		return ErrEdgeNil
	}
	if len(edge.GetId()) == 0 {
		return ErrEdgeEmptyId
	}
	if len(edge.GetSourceNodeId()) == 0 {
		return ErrEdgeEmptySourceNodeId
	}
	if len(edge.GetTargetNodeId()) == 0 {
		return ErrEdgeEmptyTargetNodeId
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *CanvasAddEdgeOp) GetOperationTypeId() string {
	return CanvasAddEdgeOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasAddEdgeOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	edge := o.GetEdge()

	objState, found, err := worldHandle.GetObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if !found {
		return false, world.ErrObjectNotFound
	}

	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		state, uerr := s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		if uerr != nil {
			return uerr
		}
		if state == nil {
			state = &s4wave_canvas.CanvasState{}
		}

		// Verify source and target nodes exist.
		if state.GetNodes() == nil {
			return ErrEdgeNodeNotFound
		}
		if _, ok := state.GetNodes()[edge.GetSourceNodeId()]; !ok {
			return ErrEdgeNodeNotFound
		}
		if _, ok := state.GetNodes()[edge.GetTargetNodeId()]; !ok {
			return ErrEdgeNodeNotFound
		}

		state.Edges = append(state.Edges, edge)
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CanvasAddEdgeOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CanvasAddEdgeOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CanvasAddEdgeOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCanvasAddEdgeOp looks up a CanvasAddEdgeOp operation type.
func LookupCanvasAddEdgeOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CanvasAddEdgeOpId {
		return &CanvasAddEdgeOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CanvasAddEdgeOp)(nil))
