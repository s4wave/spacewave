package s4wave_device

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// ComputersDashboardTypeID is the object type identifier for Computers dashboards.
const ComputersDashboardTypeID = "spacewave/computers"

// UnmarshalComputersDashboard unmarshals a ComputersDashboard from a cursor.
func UnmarshalComputersDashboard(ctx context.Context, bcs *block.Cursor) (*ComputersDashboard, error) {
	return block.UnmarshalBlock[*ComputersDashboard](ctx, bcs, func() block.Block {
		return &ComputersDashboard{}
	})
}

// GetBlockTypeId returns the block type identifier.
func (d *ComputersDashboard) GetBlockTypeId() string {
	return ComputersDashboardTypeID
}

// MarshalBlock marshals the block to binary.
func (d *ComputersDashboard) MarshalBlock() ([]byte, error) {
	return d.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (d *ComputersDashboard) UnmarshalBlock(data []byte) error {
	return d.UnmarshalVT(data)
}

// Validate performs cursory checks on the ComputersDashboard.
func (d *ComputersDashboard) Validate() error {
	return nil
}
