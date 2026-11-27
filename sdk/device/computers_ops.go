package s4wave_device

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewCreateComputersDashboardOp constructs a new CreateComputersDashboardOp.
func NewCreateComputersDashboardOp(objKey, name string, ts time.Time) *CreateComputersDashboardOp {
	return &CreateComputersDashboardOp{
		ObjectKey: objKey,
		Name:      name,
		Timestamp: timestamppb.New(ts),
	}
}

// Validate checks the op cursorily.
func (o *CreateComputersDashboardOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateComputersDashboardOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the op and build the dashboard record.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Build the dashboard record from the op fields.
	objKey := o.GetObjectKey()
	dashboard := &ComputersDashboard{
		Name:      o.GetName(),
		CreatedAt: o.GetTimestamp(),
	}

	// Create the dashboard World object and set its object type.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(dashboard, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Set the dashboard's object type.
	if err := world_types.SetObjectType(ctx, ws, objKey, ComputersDashboardTypeID); err != nil {
		return false, err
	}

	return false, nil
}

var _ world.Operation = (*CreateComputersDashboardOp)(nil)
