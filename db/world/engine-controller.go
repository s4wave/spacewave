package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
)

// EngineController lends an existing Engine to a bus under one exact identifier.
// The caller owns the Engine and must retain it until the controller is removed.
type EngineController struct {
	id     string
	engine Engine
}

// NewEngineController exposes an already-authorized Engine without reopening storage.
func NewEngineController(id string, engine Engine) *EngineController {
	return &EngineController{id: id, engine: engine}
}

// GetControllerInfo identifies this borrowed Engine mount.
func (c *EngineController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("world/engine/"+c.id, controller.MustParseVersion("0.0.1"), "borrowed World engine")
}

// GetWorldEngine returns the capability supplied by the owner.
func (c *EngineController) GetWorldEngine(context.Context) (Engine, error) {
	return c.engine, nil
}

// HandleDirective resolves only the mounted identifier, avoiding ambient World selection.
func (c *EngineController) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if lookup, ok := di.GetDirective().(LookupWorldEngine); ok && lookup.LookupWorldEngineID() == c.id {
		return directive.R(NewWorldEngineResolver(c))
	}
	return nil, nil
}

// Execute leaves the borrowed capability available for the controller lifetime.
func (c *EngineController) Execute(context.Context) error { return nil }

// Close releases no storage; the caller retains ownership of the Engine.
func (c *EngineController) Close() error { return nil }

var _ Controller = (*EngineController)(nil)
