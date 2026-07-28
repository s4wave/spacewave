package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

type objectBodyWorldState struct {
	world.WorldState
}

// GetObjectBodiesBatchPage returns one budgeted page from this world state.
func (t *WorldState) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, error) {
	return world.GetObjectBodiesBatchPage(ctx, objectBodyWorldState{WorldState: t}, keys, byteBudget)
}

// GetObjectBodiesBatchPageWithSeqno returns one budgeted page and its transaction seqno.
func (t *WorldState) GetObjectBodiesBatchPageWithSeqno(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, uint64, error) {
	seqno, err := t.GetSeqno(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	bodies, consumed, err := t.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
	return bodies, consumed, seqno, err
}

var _ world.ObjectBodyPageBatcher = ((*WorldState)(nil))
var _ world.ObjectBodyPageSeqnoBatcher = ((*WorldState)(nil))
