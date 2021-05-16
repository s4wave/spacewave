package execution_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
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
	ProcessTransaction(tx execution_transaction.Transaction) error
	// GetExecutionState waits for the latest execution state and returns it.
	// This MUST reflect the changes applied by ProcessTransaction to avoid loops.
	GetExecutionState() (*block.Cursor, error)
}
