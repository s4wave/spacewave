package s4wave_camera

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
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
	return validateCameraFields(c.GetName(), c.GetDeviceObjectKey(), c.GetCapabilityId())
}

// WorldTitle returns the name, which object browsers show for the object.
func (c *Camera) WorldTitle() string {
	return c.GetName()
}

// validateCameraFields requires the fields a Camera and its create op share.
func validateCameraFields(name, deviceObjectKey, capabilityID string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("camera name is required")
	}
	if strings.TrimSpace(deviceObjectKey) == "" {
		return errors.New("camera device object key is required")
	}
	if strings.TrimSpace(capabilityID) == "" {
		return errors.New("camera capability id is required")
	}
	return nil
}

// _ is a type assertion
var _ block.Block = (*Camera)(nil)

// _ is a type assertion
var _ world.TitledBlock = (*Camera)(nil)
