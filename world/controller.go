package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
)

// Controller is a Hydra World controller managing an Engine.
type Controller interface {
	// Controller indicates this is a controller-bus controller.
	controller.Controller
	// GetWorldEngine waits for the engine to be built.
	// Returns a new EngineHandle, be sure to call Release when done.
	GetWorldEngine(context.Context) (EngineHandle, error)
}
