package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetObjectRootRefsBatch returns object root refs for object keys.
func (e *EngineTx) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	return performOpValue(ctx, e, func(tx *Tx) ([]*world.ObjectRootRef, error) {
		return tx.GetObjectRootRefsBatch(ctx, keys)
	})
}

// _ is a type assertion
var _ world.ObjectRootRefBatcher = (*EngineTx)(nil)
