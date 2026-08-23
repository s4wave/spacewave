package statusprojector

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"

	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// ControllerID is the controller identifier.
const ControllerID = "resource/desktop/status-projector"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

var controllerDescrip = "desktop runtime status projector controller"

// Controller publishes Spacewave runtime status into the desktop runtime tree.
type Controller struct {
	*bus.BusController[*Config]

	// statusBroker is shared with the resource listener controller through
	// the composition root so the tray projection reads the same listener
	// state the controller publishes.
	statusBroker *resource_listener.StatusBroker
}

// Option configures the status projector controller factory.
type Option func(*Controller)

// WithListenerStatusBroker injects the shared listener status broker.
func WithListenerStatusBroker(broker *resource_listener.StatusBroker) Option {
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
