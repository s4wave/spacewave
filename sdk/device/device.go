package s4wave_device

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// DeviceTypeID is the type identifier for Spacewave-managed Device objects.
const DeviceTypeID = "spacewave/device"

// NewDeviceBlock constructs a new Device block.
func NewDeviceBlock() block.Block {
	return &Device{}
}

// UnmarshalDevice unmarshals a Device from a cursor.
func UnmarshalDevice(ctx context.Context, bcs *block.Cursor) (*Device, error) {
	return block.UnmarshalBlock[*Device](ctx, bcs, NewDeviceBlock)
}

// MarshalBlock marshals the Device to bytes.
func (d *Device) MarshalBlock() ([]byte, error) {
	return d.MarshalVT()
}

// UnmarshalBlock unmarshals the Device from bytes.
func (d *Device) UnmarshalBlock(data []byte) error {
	return d.UnmarshalVT(data)
}

// Validate performs cursory checks on the Device block.
func (d *Device) Validate() error {
	if strings.TrimSpace(d.GetPeerId()) == "" {
		return errors.New("device peer_id is required")
	}
	if strings.TrimSpace(d.GetLabel()) == "" {
		return errors.New("device label is required")
	}
	seenCapabilities := make(map[string]struct{}, len(d.GetCapabilities()))
	for _, cap := range d.GetCapabilities() {
		if cap == nil {
			continue
		}
		id := strings.TrimSpace(cap.GetId())
		if id == "" {
			return errors.New("device capability id is required")
		}
		if _, ok := seenCapabilities[id]; ok {
			return errors.Errorf("duplicate device capability id %q", id)
		}
		seenCapabilities[id] = struct{}{}
	}
	return nil
}

// _ is a type assertion
var _ block.Block = (*Device)(nil)
