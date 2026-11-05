package space_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
)

// ControllerID identifies the Space World engine controller.
const ControllerID = "space/world/engine"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Factory constructs Space World engine controllers.
type Factory struct {
	// bus is the controller bus.
	bus bus.Bus
	// opTypes resolves the operation types of each engine.
	opTypes OpTypes
}

// NewFactory returns a factory whose engines resolve operation types with
// opTypes before the bus.
func NewFactory(b bus.Bus, opTypes OpTypes) *Factory {
	return &Factory{bus: b, opTypes: opTypes}
}

// GetConfigID returns the configuration ID for the controller.
func (f *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the controller with the default block transforms.
func (f *Factory) Construct(_ context.Context, conf config.Config, opts controller.ConstructOpts) (controller.Controller, error) {
	return NewController(opts.GetLogger(), f.bus, conf.(*Config), transform_all.BuildFactorySet(), f.opTypes), nil
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
