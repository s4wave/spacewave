package core_unixfs_linkedfolder

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	unixfs_linkedfolder "github.com/s4wave/spacewave/sdk/unixfs/linkedfolder"
)

// Factory constructs linked-folder controllers.
type Factory struct {
	// bus is the controller bus.
	bus bus.Bus
}

// NewFactory builds a linked-folder controller factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the unique ID for the config.
func (f *Factory) GetConfigID() string {
	return unixfs_linkedfolder.ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &unixfs_linkedfolder.Config{}
}

// Construct constructs the linked-folder controller for a config.
func (f *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	return NewController(opts.GetLogger(), f.bus, conf.(*unixfs_linkedfolder.Config)), nil
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
