package forge_target

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
)

// ExecControllerHandle is the handle passed to the exec controller during init.
// This contains functions that can be called during execution.
type ExecControllerHandle interface {
	// TODO
}

// ExecController is a controller that implements the target Exec controller.
// The controller will be constructed using the exec.controller config.
type ExecController interface {
	// Controller indicates this is a controllerbus controller.
	controller.Controller
	// InitForgeExecController initializes the Forge execution controller.
	// This is called before Execute().
	// Any error returned cancels execution of the controller.
	InitForgeExecController(ctx context.Context, handle ExecControllerHandle) error
}
