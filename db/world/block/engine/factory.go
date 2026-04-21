package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/blang/semver/v4"
)

// ControllerID identifies the block graph engine controller.
const ControllerID = "hydra/world/block/engine"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// Factory constructs a world engine controller
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a world block engine factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the configuration ID for the controller.
func (t *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (t *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated controller given configuration.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	sfs := transform_all.BuildFactorySet()

	// Construct the controller.
	return NewController(le, t.bus, cc, sfs)
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
