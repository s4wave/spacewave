package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetObjectBodiesBatchPage reads one page of object bodies and the seqno of
// this world state, each body through its object.
func (t *WorldState) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, uint64, error) {
	return world.LookupObjectBodiesPage(ctx, t, keys, byteBudget)
}
