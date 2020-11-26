package reconciler_example

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	rc "github.com/aperturerobotics/hydra/reconciler/controller"
	"github.com/blang/semver"
)

// Factory constructs a example reconciler controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a example reconciler controller.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the unique ID for the config.
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
// The transport's identity (private key) comes from a GetNode lookup.
func (t *Factory) Construct(
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	// Construct the reconciler controller.
	return rc.NewController(
		le,
		t.bus,
		controller.NewInfo(
			ControllerID,
			Version,
			"example reconciler "+cc.GetReconcilerId()+" @ "+cc.GetBucketId(),
		),
		NewReconciler(le, cc),
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
