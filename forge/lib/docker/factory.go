package forge_lib_docker

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
)

// Factory constructs a docker controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
	// admission belongs to the Worker that registered this factory.
	admission Admission
}

// NewWorkerFactory binds Docker execution to one Worker's admission.
//
// The name differs from NewFactory on purpose: Bldr bundles every package that
// declares NewFactory as a bus-level factory, but this one is registered only by
// the Worker that owns the admission.
func NewWorkerFactory(bus bus.Bus, admission Admission) *Factory {
	return &Factory{bus: bus, admission: admission}
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
	return NewController(opts.GetLogger(), t.bus, conf.(*Config), t.admission), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
