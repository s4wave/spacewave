package signaling_echo

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/signaling"
)

// Version is the version of the controller.
var Version = semver.MustParse("0.0.1")

// ControllerID identifies the world object volume controller.
const ControllerID = "alpha/signaling/echo"

// controllerDescrip is the controller description.
var controllerDescrip = "echos incoming signaling messages"

// Controller implements the signaling echo controller.
//
// Resolves HandleSignalPeer by echoing incoming messages.
type Controller struct {
	*bus.BusController[*Config]
}

// NewFactory constructs the echo factory.
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
			}, nil
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if d, ok := di.GetDirective().(signaling.HandleSignalPeer); ok {
		return c.resolveHandleSignalPeer(ctx, di, d)
	}

	return nil, nil
}

// resolveHandleSignalPeer resolves a HandleSignalPeer directive by echoing data.
func (c *Controller) resolveHandleSignalPeer(
	ctx context.Context,
	di directive.Instance,
	dir signaling.HandleSignalPeer,
) ([]directive.Resolver, error) {
	res, err := NewEchoResolver(c.GetLogger(), dir)
	if err != nil {
		return nil, err
	}
	return directive.R(res, nil)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
