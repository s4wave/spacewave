//go:build !bldr_startup_trace || tinygo

package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	return t.lookupGraphQuadsOnWorld(ctx, filter, limit)
}
