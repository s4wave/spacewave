package cdn_world_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
)

// Factory constructs the CDN world controller.
type Factory struct {
	b bus.Bus
}

// NewFactory builds the factory.
func NewFactory(b bus.Bus) *Factory {
	return &Factory{b: b}
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

// Construct constructs the associated controller given configuration.
func (f *Factory) Construct(
	_ context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	cc := conf.(*Config)
	if err := cc.Validate(); err != nil {
		return nil, err
	}
	return NewController(opts.GetLogger(), f.b, cc), nil
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion.
var _ controller.Factory = (*Factory)(nil)
