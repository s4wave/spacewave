package resource_listener

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"

	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

// ControllerID is the controller identifier.
const ControllerID = "resource/listener"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "resource service unix socket listener controller"

// Controller is the resource listener controller.
type Controller struct {
	*bus.BusController[*Config]

	// yieldBroker and statusBroker are shared with the Root resource server
	// and the daemon CLI through the composition root so takeover prompts,
	// reclaim signals, and listener status agree across consumers.
	yieldBroker  *yield_policy.Broker
	statusBroker *StatusBroker
}

// Option configures the listener controller factory.
type Option func(*Controller)

// WithYieldBroker injects the shared yield broker.
func WithYieldBroker(broker *yield_policy.Broker) Option {
	return func(c *Controller) { c.yieldBroker = broker }
}

// WithStatusBroker injects the shared listener status broker.
func WithStatusBroker(broker *StatusBroker) Option {
	return func(c *Controller) { c.statusBroker = broker }
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus, opts ...Option) controller.Factory {
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
			c := &Controller{BusController: base}
			for _, opt := range opts {
				opt(c)
			}
			return c, nil
		},
	)
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
