package forge_dashboard

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateForgeDashboardOpId is the operation id for CreateForgeDashboardOp.
var CreateForgeDashboardOpId = "spacewave/forge/dashboard/create"

// LinkForgeDashboardOpId is the operation id for LinkForgeDashboardOp.
var LinkForgeDashboardOpId = "spacewave/forge/dashboard/link"

// NewCreateForgeDashboardOp constructs a new CreateForgeDashboardOp.
func NewCreateForgeDashboardOp(objKey, name string, ts time.Time) *CreateForgeDashboardOp {
	return &CreateForgeDashboardOp{
		ObjectKey: objKey,
		Name:      name,
		Timestamp: timestamppb.New(ts),
	}
}

// NewCreateForgeDashboardOpBlock constructs a new CreateForgeDashboardOp block.
func NewCreateForgeDashboardOpBlock() block.Block {
	return &CreateForgeDashboardOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateForgeDashboardOp) GetOperationTypeId() string {
	return CreateForgeDashboardOpId
}

// Validate performs cursory checks on the op.
func (o *CreateForgeDashboardOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateForgeDashboardOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	dashboard := &ForgeDashboard{
		Name:      o.GetName(),
		CreatedAt: o.GetTimestamp(),
	}

	_, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(dashboard, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, objKey, ForgeDashboardTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateForgeDashboardOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateForgeDashboardOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateForgeDashboardOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateForgeDashboardOp looks up a CreateForgeDashboardOp operation type.
func LookupCreateForgeDashboardOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateForgeDashboardOpId {
		return &CreateForgeDashboardOp{}, nil
	}
	return nil, nil
}

// NewLinkForgeDashboardOp constructs a new LinkForgeDashboardOp.
func NewLinkForgeDashboardOp(dashboardKey, entityKey string) *LinkForgeDashboardOp {
	return &LinkForgeDashboardOp{
		DashboardKey: dashboardKey,
		EntityKey:    entityKey,
	}
}

// NewLinkForgeDashboardOpBlock constructs a new LinkForgeDashboardOp block.
func NewLinkForgeDashboardOpBlock() block.Block {
	return &LinkForgeDashboardOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *LinkForgeDashboardOp) GetOperationTypeId() string {
	return LinkForgeDashboardOpId
}

// Validate performs cursory checks on the op.
func (o *LinkForgeDashboardOp) Validate() error {
	if len(o.GetDashboardKey()) == 0 {
		return errors.New("dashboard_key is required")
	}
	if len(o.GetEntityKey()) == 0 {
		return errors.New("entity_key is required")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *LinkForgeDashboardOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	gq := world.NewGraphQuadWithKeys(
		o.GetDashboardKey(),
		PredDashboardForgeRef.String(),
		o.GetEntityKey(),
		"",
	)
	if err := ws.SetGraphQuad(ctx, gq); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *LinkForgeDashboardOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *LinkForgeDashboardOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *LinkForgeDashboardOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupLinkForgeDashboardOp looks up a LinkForgeDashboardOp operation type.
func LookupLinkForgeDashboardOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == LinkForgeDashboardOpId {
		return &LinkForgeDashboardOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var (
	_ world.Operation = ((*CreateForgeDashboardOp)(nil))
	_ world.Operation = ((*LinkForgeDashboardOp)(nil))
)
