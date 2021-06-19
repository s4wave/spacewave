package world

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
)

// Operation is a batch operation against World or Object.
type Operation interface {
	// Block indicates this operation is serializable to a block.
	block.Block
}

// ApplyWorldOpFunc executes a custom world operation type.
// Returns false, nil if unable to handle this operation type.
type ApplyWorldOpFunc = func(
	ctx context.Context,
	worldHandle WorldState,
	operationTypeID string,
	op Operation,
) (handled bool, err error)

// ApplyObjectOpFunc executes a custom object operation type.
// Returns false, nil if unable to handle this operation type.
type ApplyObjectOpFunc = func(
	ctx context.Context,
	objectHandle ObjectState,
	operationTypeID string,
	op Operation,
) (handled bool, err error)
