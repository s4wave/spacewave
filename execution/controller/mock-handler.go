package execution_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	execution_transaction "github.com/aperturerobotics/forge/execution/transaction"
	"github.com/aperturerobotics/hydra/block"
)

// MockHandler implements a mock controller handler.
type MockHandler struct {
}

// NewMockHandler constructs a new handler.
func NewMockHandler() *MockHandler {
	return &MockHandler{}
}

// CheckExecControllerConfig checks if the config is allowed.
// any error returned will cancel the execution of the controller.
func (h *MockHandler) CheckExecControllerConfig(ctx context.Context, c config.Config) error {
	// no-op
	return nil
}

// ProcessTransaction processes a transaction against the Execution.
// Waits for the transaction to be applied before returning.
func (h *MockHandler) ProcessTransaction(tx execution_transaction.Transaction) error {
	return nil
}

// GetExecutionState waits for the latest execution state and returns it.
// This MUST reflect the changes applied by ProcessTransaction to avoid loops.
func (h *MockHandler) GetExecutionState() (*block.Cursor, error) {
	return nil, nil
}

// _ is a type assertion
var _ Handler = ((*MockHandler)(nil))
