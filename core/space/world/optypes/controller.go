package optypes

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
)

// Version is the component version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "resolves common space world ops"

// Controller is the world ops controller.
type Controller struct {
	*bus.BusController[*space_world_ops.Config]
}

// NewFactory constructs the controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		space_world_ops.ConfigID,
		space_world_ops.ControllerID,
		Version,
		controllerDescrip,
		func() *space_world_ops.Config {
			return &space_world_ops.Config{}
		},
		func(base *bus.BusController[*space_world_ops.Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case world.LookupWorldOp:
		limitEngineID := c.GetConfig().GetEngineId()
		if limitEngineID != "" && d.LookupWorldOpEngineID() != limitEngineID {
			return nil, nil
		}
		return directive.R(world.NewLookupWorldOpResolver(LookupWorldOp), nil)
	}

	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
