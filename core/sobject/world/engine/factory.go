package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/world"
)

// ControllerID identifies the block graph engine controller.
const ControllerID = "sobject/world/engine"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Factory constructs a world engine controller
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
	// lookupOp resolves built-in operations before the bus lookup, if set.
	lookupOp world.LookupOp
}

// NewFactory builds a world block engine factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// NewFactoryWithLookupOp builds a world block engine factory whose engines
// resolve operations with lookupOp before the bus lookup. The lookup is set at
// construction, so it covers the first replay.
func NewFactoryWithLookupOp(bus bus.Bus, lookupOp world.LookupOp) *Factory {
	return &Factory{bus: bus, lookupOp: lookupOp}
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
	// Construct the controller with the default block transforms.
	le := opts.GetLogger()
	cc := conf.(*Config)
	sfs := transform_all.BuildFactorySet()
	ctrl, err := NewController(le, t.bus, cc, sfs)
	if err != nil {
		return nil, err
	}

	// Set the built-in operations before the controller executes.
	ctrl.SetStaticLookupOp(t.lookupOp)
	return ctrl, nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
