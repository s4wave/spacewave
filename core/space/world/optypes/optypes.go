//go:build !tinygo && !goscript

package optypes

import (
	"context"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
)

// LookupWorldOp looks up the available world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		space_world_ops.LookupWorldOp,
		lookupCoreWorldOp,
		s4wave_apt.LookupAptOp,
	}).LookupOp(ctx, opTypeID)
}
