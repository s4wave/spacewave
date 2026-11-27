package s4wave_device

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// UnmarshalComputersDashboard unmarshals a ComputersDashboard from a cursor.
func UnmarshalComputersDashboard(ctx context.Context, bcs *block.Cursor) (*ComputersDashboard, error) {
	return block.UnmarshalBlock[ComputersDashboard](ctx, bcs)
}

// Validate checks the ComputersDashboard cursorily.
func (d *ComputersDashboard) Validate() error {
	return nil
}
