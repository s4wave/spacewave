//go:build !js

package bldr_manifest_builder_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
)

// Factory constructs a bldr manifest builder controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
	// pluginBuildLimiter bounds concurrent whole-plugin build attempts
	pluginBuildLimiter *PluginBuildLimiter
	// pluginBuildLimiterErr is returned when the configured limit is invalid
	pluginBuildLimiterErr error
}

// NewFactory builds the controller factory.
func NewFactory(bus bus.Bus) *Factory {
	pluginBuildLimiter, pluginBuildLimiterErr := NewPluginBuildLimiterFromEnv()
	return &Factory{
		bus:                   bus,
		pluginBuildLimiter:    pluginBuildLimiter,
		pluginBuildLimiterErr: pluginBuildLimiterErr,
	}
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
	if t.pluginBuildLimiterErr != nil {
		return nil, t.pluginBuildLimiterErr
	}

	return newController(le, t.bus, cc, t.pluginBuildLimiter), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
