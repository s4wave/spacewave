//go:build goscript

package optypes

import (
	"context"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupWorldOp looks up the GoScript-supported world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		space_world_ops.LookupSetSpaceSettingsOp,
		space_world_ops.LookupInitUnixFSOp,
		space_world_ops.LookupInitObjectLayoutOp,
		space_world_ops.LookupInitCanvasDemoOp,
		space_world_ops.LookupCanvasInitOp,
		space_world_ops.LookupCanvasAddNodeOp,
		space_world_ops.LookupCanvasRemoveNodeOp,
		space_world_ops.LookupCanvasSetNodeOp,
		space_world_ops.LookupCanvasAddEdgeOp,
		space_world_ops.LookupCanvasRemoveEdgeOp,
		s4wave_device.LookupCreateComputersDashboardOp,
		s4wave_wizard.LookupCreateWizardObjectOp,
	}).LookupOp(ctx, opTypeID)
}
