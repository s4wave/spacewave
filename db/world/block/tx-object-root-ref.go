package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetObjectRootRefsBatch returns object root refs for object keys.
func (t *Tx) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	return lockedValue(ctx, t, false, func() ([]*world.ObjectRootRef, error) {
		return t.state.GetObjectRootRefsBatch(ctx, keys)
	})
}

// _ is a type assertion
var _ world.ObjectRootRefBatcher = (*Tx)(nil)
