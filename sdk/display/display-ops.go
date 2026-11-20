package s4wave_display

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_frametrack "github.com/s4wave/spacewave/sdk/frametrack"
	"github.com/sirupsen/logrus"
)

// Validate requires the object key, the Display's fields and the timestamp.
func (o *CreateDisplayOp) Validate() error {
	// Require the key the Display is created at.
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}

	// Require the Display's name, Device and capability, and its timestamp.
	if err := s4wave_frametrack.ValidateTarget(o.GetName(), o.GetDeviceObjectKey(), o.GetCapabilityId()); err != nil {
		return err
	}
	return o.GetTimestamp().Validate(false)
}

// ApplyWorldOp creates the Display object and sets its type.
func (o *CreateDisplayOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the request before changing World state.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create the Display object with its block.
	display := &Display{
		Name:            o.GetName(),
		DeviceObjectKey: o.GetDeviceObjectKey(),
		CapabilityId:    o.GetCapabilityId(),
		CreatedAt:       o.GetTimestamp(),
	}
	createdObject, _, err := world.CreateWorldObject(ctx, ws, o.GetObjectKey(), func(bcs *block.Cursor) error {
		bcs.SetBlock(display, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Assign the Display object type to the new object.
	return false, world_types.SetObjectType(ctx, ws, o.GetObjectKey(), DisplayTypeID)
}

// _ is a type assertion
var _ world.Operation = (*CreateDisplayOp)(nil)
