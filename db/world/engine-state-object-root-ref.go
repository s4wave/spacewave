package world

import "context"

// GetObjectRootRefsBatch returns object root refs for object keys.
func (e *engineWorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*ObjectRootRef, error) {
	var refs []*ObjectRootRef
	err := e.performOp(ctx, false, func(tx Tx) error {
		var berr error
		refs, berr = GetObjectRootRefsBatch(ctx, tx, keys)
		return berr
	})
	return refs, err
}

// _ is a type assertion
var _ ObjectRootRefBatcher = ((*engineWorldState)(nil))
