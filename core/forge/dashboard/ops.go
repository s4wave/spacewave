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

// NewCreateForgeDashboardOp constructs a new CreateForgeDashboardOp.
func NewCreateForgeDashboardOp(objKey, name string, ts time.Time) *CreateForgeDashboardOp {
	return &CreateForgeDashboardOp{
		ObjectKey: objKey,
		Name:      name,
		Timestamp: timestamppb.New(ts),
	}
}

// Validate cursorily checks the op.
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
	// Validate the operation before creating any world objects.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Read the target object key and build the dashboard record.
	objKey := o.GetObjectKey()
	dashboard := &ForgeDashboard{
		Name:      o.GetName(),
		CreatedAt: o.GetTimestamp(),
	}

	// Create the dashboard object and release its handle.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(dashboard, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Record the object type for the dashboard object.
	if err := world_types.SetObjectType(ctx, ws, objKey, ForgeDashboardTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// NewLinkForgeDashboardOp constructs a new LinkForgeDashboardOp.
func NewLinkForgeDashboardOp(dashboardKey, entityKey string) *LinkForgeDashboardOp {
	return &LinkForgeDashboardOp{
		DashboardKey: dashboardKey,
		EntityKey:    entityKey,
	}
}

// Validate cursorily checks the op.
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

// _ is a type assertion
var (
	_ world.Operation = (*CreateForgeDashboardOp)(nil)
	_ world.Operation = (*LinkForgeDashboardOp)(nil)
)
