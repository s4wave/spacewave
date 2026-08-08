package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// GetObjectRootRefsBatch returns object root refs for object keys.
func (t *Tx) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	unlock, err := t.rmtx.Lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer unlock()

	if t.state.discarded.Load() {
		return nil, tx.ErrDiscarded
	}
	return t.state.GetObjectRootRefsBatch(ctx, keys)
}

// _ is a type assertion
var _ world.ObjectRootRefBatcher = (*Tx)(nil)
