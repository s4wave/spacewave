package s4wave_camera

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_frametrack "github.com/s4wave/spacewave/sdk/frametrack"
)

// NewCameraBlock constructs a new Camera block.
func NewCameraBlock() block.Block {
	return &Camera{}
}

// UnmarshalCamera unmarshals a Camera from a cursor.
func UnmarshalCamera(ctx context.Context, bcs *block.Cursor) (*Camera, error) {
	return block.UnmarshalBlock[Camera](ctx, bcs)
}

// Validate requires the name, the Device object key and the capability ID.
func (c *Camera) Validate() error {
	return s4wave_frametrack.ValidateTarget(c.GetName(), c.GetDeviceObjectKey(), c.GetCapabilityId())
}

// WorldTitle returns the name, which object browsers show for the object.
func (c *Camera) WorldTitle() string {
	return c.GetName()
}

// _ is a type assertion
var _ block.Block = (*Camera)(nil)

// _ is a type assertion
var _ world.TitledBlock = (*Camera)(nil)
