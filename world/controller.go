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
	GetWorldEngine(context.Context) (Engine, error)
}
