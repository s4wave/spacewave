package s4wave_flowgraph

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateFlowgraphOpID identifies the World operation that creates a Flowgraph.
const CreateFlowgraphOpID = "flowgraph/create"

// GetOperationTypeId returns the operation's wire identifier.
func (o *CreateFlowgraphOp) GetOperationTypeId() string {
	return CreateFlowgraphOpID
}

// Validate requires an independently addressed Flowgraph key.
func (o *CreateFlowgraphOp) Validate() error {
	_, err := ParseFlowgraphObjectKey(o.GetObjectKey())
	return err
}

// ApplyWorldOp creates the authored body and its type edge in the caller's transaction.
func (o *CreateFlowgraphOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	// Require a valid identity before creating the object.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Store the initial authored body and release the returned object handle.
	obj, _, err := world.CreateWorldObject(ctx, ws, o.GetObjectKey(), func(cursor *block.Cursor) error {
		cursor.SetBlock(&Flowgraph{Name: o.GetName()}, true)
		return nil
	})
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}

	// Register the object's type through the World graph.
	return false, world_types.SetObjectType(ctx, ws, o.GetObjectKey(), FlowgraphTypeID)
}

// ApplyWorldObjectOp rejects an operation that requires the enclosing World.
func (o *CreateFlowgraphOp) ApplyWorldObjectOp(context.Context, *logrus.Entry, world.ObjectState, peer.ID) (bool, error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock encodes the creation request.
func (o *CreateFlowgraphOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock decodes the creation request.
func (o *CreateFlowgraphOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateFlowgraphOp resolves the Flowgraph creation operation.
func LookupCreateFlowgraphOp(_ context.Context, id string) (world.Operation, error) {
	if id == CreateFlowgraphOpID {
		return &CreateFlowgraphOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion.
var _ world.Operation = (*CreateFlowgraphOp)(nil)
