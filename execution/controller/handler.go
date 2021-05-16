package execution_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/forge/execution/transaction"
	"github.com/aperturerobotics/hydra/block"
)

// Handler manages an Execution controller.
// Performs get/set operations against the exec state.
type Handler interface {
	// CheckExecControllerConfig checks if the config is allowed.
	// any error returned will cancel the execution of the controller.
	CheckExecControllerConfig(ctx context.Context, c config.Config) error
	// ProcessTransaction processes a transaction against the Execution.
	// Waits for the transaction to be applied before returning.
	// Returns the revision of the state with the tx included.
	ProcessTransaction(tx execution_transaction.Transaction) (uint64, error)
	// WaitExecutionState waits for an execution state with revision >= rev and returns it.
	// If rev == 0, returns any revision.
	WaitExecutionState(rev uint64) (*forge_execution.Execution, *block.Cursor, error)
}
