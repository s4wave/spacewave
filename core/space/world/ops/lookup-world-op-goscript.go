//go:build goscript

package space_world_ops

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// LookupWorldOp looks up the GoScript-supported built-in space world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		LookupSetSpaceSettingsOp,
		LookupInitUnixFSOp,
		LookupInitObjectLayoutOp,
		LookupInitCanvasDemoOp,
		LookupCanvasInitOp,
		LookupCanvasAddNodeOp,
		LookupCanvasRemoveNodeOp,
		LookupCanvasSetNodeOp,
		LookupCanvasAddEdgeOp,
		LookupCanvasRemoveEdgeOp,
	}).LookupOp(ctx, opTypeID)
}
