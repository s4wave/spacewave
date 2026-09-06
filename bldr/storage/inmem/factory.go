package storage_inmem

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
)

// Factory constructs the in-memory storage controller.
type Factory struct{}

// NewFactory exposes process-local storage to config-driven runtimes.
func NewFactory(_ bus.Bus) *Factory {
	return &Factory{}
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
func (t *Factory) Construct(
	_ context.Context,
	conf config.Config,
	_ controller.ConstructOpts,
) (controller.Controller, error) {
	cc, ok := conf.(*Config)
	if !ok {
		return nil, errors.Errorf("expected config type %T, got %T", &Config{}, conf)
	}
	if err := cc.Validate(); err != nil {
		return nil, err
	}

	return NewController(cc.GetStorageId()), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
