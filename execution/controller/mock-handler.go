package execution_controller

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_transaction "github.com/aperturerobotics/forge/execution/transaction"
	"github.com/aperturerobotics/hydra/block"
)

// MockHandler implements a mock controller handler.
type MockHandler struct {
	// internally mock the state
	currState *forge_execution.Execution
	// wakeCh is the wake channel
	wakeCh chan struct{}
}

// NewMockHandler constructs a new handler.
func NewMockHandler(initState *forge_execution.Execution) *MockHandler {
	if initState == nil {
		initState = &forge_execution.Execution{}
	}
	wakeCh := make(chan struct{})
	return &MockHandler{currState: initState, wakeCh: wakeCh}
}

// CheckExecControllerConfig checks if the config is allowed.
// any error returned will cancel the execution of the controller.
func (h *MockHandler) CheckExecControllerConfig(ctx context.Context, c config.Config) error {
	// no-op
	return nil
}

// ProcessTransaction processes a transaction against the Execution.
// Waits for the transaction to be applied before returning.
// Returns the revision of the state with the tx included.
func (h *MockHandler) ProcessTransaction(tx execution_transaction.Transaction) (uint64, error) {
	return 0, errors.New("TODO process transaction")
}

// WaitExecutionState waits for an execution state with revision > rev and returns it.
// If rev == 0, returns any revision.
func (h *MockHandler) WaitExecutionState(rev uint64) (*forge_execution.Execution, *block.Cursor, error) {

	return nil, nil, errors.New("TODO wait execution state")
}

// _ is a type assertion
var _ Handler = ((*MockHandler)(nil))
