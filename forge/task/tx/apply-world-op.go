package task_tx

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// LookupWorldOp performs the lookup operation for the Task op types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == WorldOperationTypeID {
		return &Tx{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupWorldOp
