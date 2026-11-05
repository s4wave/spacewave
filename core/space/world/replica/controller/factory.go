package space_world_replica_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	space_world_replica "github.com/s4wave/spacewave/core/space/world/replica"
)

// Factory constructs replica controllers. It belongs on a Session's bus, where
// the Session peer and its links are.
type Factory struct {
	// bus is the Session's bus.
	bus bus.Bus
}

// NewFactory builds a replica controller factory for a Session's bus.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the unique ID for the config.
func (f *Factory) GetConfigID() string {
	return space_world_replica.ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &space_world_replica.Config{}
}

// Construct constructs the replica controller for a config.
func (f *Factory) Construct(
	_ context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	return NewController(opts.GetLogger(), f.bus, conf.(*space_world_replica.Config))
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
