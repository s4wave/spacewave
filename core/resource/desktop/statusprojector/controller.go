package statusprojector

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)

// ControllerID is the controller identifier.
const ControllerID = "resource/desktop/status-projector"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

var controllerDescrip = "desktop runtime status projector controller"

// Controller publishes Spacewave runtime status into the desktop runtime tree.
type Controller struct {
	*bus.BusController[*Config]
}

// NewFactory constructs the component factory.
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
			return &Controller{BusController: base}, nil
		},
	)
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
