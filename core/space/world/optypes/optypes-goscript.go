//go:build goscript

package optypes

import (
	"context"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
)

// LookupWorldOp looks up the GoScript-supported world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		space_world_ops.LookupWorldOp,
		lookupCoreWorldOp,
	}).LookupOp(ctx, opTypeID)
}
