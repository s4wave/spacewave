package s4wave_camera

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_frametrack "github.com/s4wave/spacewave/sdk/frametrack"
	"github.com/sirupsen/logrus"
)

// Validate requires the object key, the Camera's fields and the timestamp.
func (o *CreateCameraOp) Validate() error {
	// Require the key the Camera is created at.
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}

	// Require the Camera's name, Device and capability, and its timestamp.
	if err := s4wave_frametrack.ValidateTarget(o.GetName(), o.GetDeviceObjectKey(), o.GetCapabilityId()); err != nil {
		return err
	}
	return o.GetTimestamp().Validate(false)
}

// ApplyWorldOp creates the Camera object and sets its type.
func (o *CreateCameraOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the request before changing World state.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create the Camera object with its block.
	camera := &Camera{
		Name:            o.GetName(),
		DeviceObjectKey: o.GetDeviceObjectKey(),
		CapabilityId:    o.GetCapabilityId(),
		CreatedAt:       o.GetTimestamp(),
	}
	createdObject, _, err := world.CreateWorldObject(ctx, ws, o.GetObjectKey(), func(bcs *block.Cursor) error {
		bcs.SetBlock(camera, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Assign the Camera object type to the new object.
	return false, world_types.SetObjectType(ctx, ws, o.GetObjectKey(), CameraTypeID)
}

// _ is a type assertion
var _ world.Operation = (*CreateCameraOp)(nil)
