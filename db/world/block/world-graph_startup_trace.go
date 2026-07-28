//go:build bldr_startup_trace && !tinygo

package world_block

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
	"github.com/s4wave/spacewave/db/world"
)

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "hydra/world-block/world-state/lookup-graph-quads")
	defer task.End()
	return t.lookupGraphQuadsOnWorld(traceCtx, filter, limit)
}
