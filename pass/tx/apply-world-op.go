package pass_tx

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
)

// LookupWorldOp performs the lookup operation for the pass op types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == WorldOperationTypeID {
		return &Tx{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupWorldOp
