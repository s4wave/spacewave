package blocktype_controller_factory

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	space_world "github.com/s4wave/spacewave/core/space/world"
	blocktype_controller "github.com/s4wave/spacewave/hydra-exp/blocktype/controller"
)

// ControllerID is the controller id.
const ControllerID = "hydra-exp/blocktype"

// Version is the component version.
var Version = semver.MustParse("1.0.0")

// controllerDescrip is the controller description.
var controllerDescrip = "resolves block type lookups"

// Controller is the blocktype controller.
type Controller struct {
	*bus.BusController[*Config]
	btController *blocktype_controller.Controller
}

// NewFactory constructs the controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
				btController:  blocktype_controller.NewController(space_world.LookupBlockType),
			}, nil
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return c.btController.HandleDirective(ctx, di)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
