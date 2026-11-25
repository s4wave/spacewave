package os_system_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	os_system "github.com/s4wave/spacewave/os/system"
)

// Factory constructs system controllers.
type Factory struct {
	// bus is the controller bus.
	bus bus.Bus
}

// NewFactory builds a system controller factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the unique ID for the config.
func (f *Factory) GetConfigID() string {
	return os_system.ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &os_system.Config{}
}

// Construct constructs the system controller for a config.
func (f *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	return NewController(opts.GetLogger(), f.bus, conf.(*os_system.Config)), nil
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
