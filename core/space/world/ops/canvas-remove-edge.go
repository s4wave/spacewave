package space_world_ops

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/sirupsen/logrus"
)

// CanvasRemoveEdgeOpId is the operation id for CanvasRemoveEdgeOp.
var CanvasRemoveEdgeOpId = "space/world/canvas-remove-edge"

// NewCanvasRemoveEdgeOp constructs a new CanvasRemoveEdgeOp block.
func NewCanvasRemoveEdgeOp(objKey string, edgeIDs []string) *CanvasRemoveEdgeOp {
	return &CanvasRemoveEdgeOp{
		ObjectKey: objKey,
		EdgeIds:   edgeIDs,
	}
}

// NewCanvasRemoveEdgeOpBlock constructs a new CanvasRemoveEdgeOp block.
func NewCanvasRemoveEdgeOpBlock() block.Block {
	return &CanvasRemoveEdgeOp{}
}

// Validate performs cursory checks on the op.
func (o *CanvasRemoveEdgeOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(o.GetEdgeIds()) == 0 {
		return ErrNoEdgeIds
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *CanvasRemoveEdgeOp) GetOperationTypeId() string {
	return CanvasRemoveEdgeOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CanvasRemoveEdgeOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	edgeIDs := o.GetEdgeIds()

	objState, found, err := worldHandle.GetObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if !found {
		return false, world.ErrObjectNotFound
	}

	// Build a set of IDs to remove.
	removeSet := make(map[string]struct{}, len(edgeIDs))
	for _, id := range edgeIDs {
		removeSet[id] = struct{}{}
	}

	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		state, uerr := s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		if uerr != nil {
			return uerr
		}
		if state == nil {
			state = &s4wave_canvas.CanvasState{}
		}

		// Filter edges to exclude removed IDs.
		edges := state.GetEdges()
		filtered := make([]*s4wave_canvas.CanvasEdge, 0, len(edges))
		for _, e := range edges {
			if _, remove := removeSet[e.GetId()]; !remove {
				filtered = append(filtered, e)
			}
		}
		state.Edges = filtered

		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CanvasRemoveEdgeOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CanvasRemoveEdgeOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CanvasRemoveEdgeOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCanvasRemoveEdgeOp looks up a CanvasRemoveEdgeOp operation type.
func LookupCanvasRemoveEdgeOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CanvasRemoveEdgeOpId {
		return &CanvasRemoveEdgeOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CanvasRemoveEdgeOp)(nil))
