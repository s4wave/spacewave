package unixfs_access_http

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
)

// Factory constructs a controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a factory.
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
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	// le := opts.GetLogger()
	cc := conf.(*Config)

	pathRe, err := cc.ParsePathRe()
	if err != nil {
		return nil, err
	}

	// Construct the controller.
	return NewController(
		t.bus,
		controller.NewInfo(ControllerID, Version, "exposes unixfs to http"),
		cc.GetMatchPathPrefixes(),
		cc.GetStripPathPrefix(),
		pathRe,
		cc.GetUnixfsId(),
		cc.GetUnixfsPrefix(),
		cc.GetUnixfsHttpPrefix(),
		cc.GetNotFoundIfIdle(),
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
