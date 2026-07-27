//go:build !bldr_startup_trace || tinygo

package world

import "context"

func (e *engineWorldState) newOperationTransaction(ctx context.Context, write bool) (Tx, error) {
	return e.e.NewTransaction(ctx, write)
}

func discardOperationTransaction(_ context.Context, tx Tx) {
	tx.Discard()
}
