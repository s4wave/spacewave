package execution_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
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

// _ is a type assertion
var _ Handler = ((*MockHandler)(nil))
