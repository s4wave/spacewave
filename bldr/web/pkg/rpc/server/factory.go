package web_pkg_rpc_server

import (
	"github.com/aperturerobotics/controllerbus/bus"
)

// Factory constructs a controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewFactory builds the controller factory.
func NewFactory(b bus.Bus) *Factory {
	return bus.NewBusFactory(
		b,
		ConfigID,
		Version,
		func() *Config { return &Config{} },
		NewController,
	)
}
