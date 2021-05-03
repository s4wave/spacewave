package execution_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
)

// Handler manages an Execution controller.
// Performs get/set operations against the exec state.
type Handler interface {
	// CheckExecControllerConfig checks if the config is allowed.
	// any error returned will cancel the execution of the controller.
	CheckExecControllerConfig(ctx context.Context, c config.Config) error
}
