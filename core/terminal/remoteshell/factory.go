//go:build !js

package remoteshell

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
)

// Factory constructs the remote shell controller on a Session bus.
type Factory struct {
	// bus is the Session bus the controller serves.
	bus bus.Bus
}

// NewFactory builds a remote shell factory for the Session bus b.
func NewFactory(b bus.Bus) *Factory {
	return &Factory{bus: b}
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

// Construct constructs the controller given its configuration.
func (f *Factory) Construct(
	_ context.Context,
	_ config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger().WithField("controller", ControllerID)
	return newController(le, f.bus), nil
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() controller.Version {
	return deviceRemoteShellControllerVersion
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
