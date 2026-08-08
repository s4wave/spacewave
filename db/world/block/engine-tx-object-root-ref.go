package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetObjectRootRefsBatch returns object root refs for object keys.
func (e *EngineTx) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	var refs []*world.ObjectRootRef
	err := e.performOp(ctx, func(tx *Tx) error {
		var berr error
		refs, berr = tx.GetObjectRootRefsBatch(ctx, keys)
		return berr
	})
	return refs, err
}

// _ is a type assertion
var _ world.ObjectRootRefBatcher = (*EngineTx)(nil)
