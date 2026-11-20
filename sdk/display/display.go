package s4wave_display

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_frametrack "github.com/s4wave/spacewave/sdk/frametrack"
)

// NewDisplayBlock constructs a new Display block.
func NewDisplayBlock() block.Block {
	return &Display{}
}

// UnmarshalDisplay unmarshals a Display from a cursor.
func UnmarshalDisplay(ctx context.Context, bcs *block.Cursor) (*Display, error) {
	return block.UnmarshalBlock[Display](ctx, bcs)
}

// Validate requires the name, the Device object key and the capability ID.
func (c *Display) Validate() error {
	return s4wave_frametrack.ValidateTarget(c.GetName(), c.GetDeviceObjectKey(), c.GetCapabilityId())
}

// WorldTitle returns the name, which object browsers show for the object.
func (c *Display) WorldTitle() string {
	return c.GetName()
}

// _ is a type assertion
var _ block.Block = (*Display)(nil)

// _ is a type assertion
var _ world.TitledBlock = (*Display)(nil)
