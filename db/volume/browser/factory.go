//go:build js

package volume_browser

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/volume"
	vc "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
)

// Factory constructs a browser volume.
type Factory struct {
	// bus is the controller bus.
	bus bus.Bus
}

// NewFactory builds a browser volume factory.
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

// Construct constructs the volume controller for conf.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	cc := conf.(*Config)
	return vc.NewController(
		opts.GetLogger(),
		cc.GetVolumeConfig(),
		t.bus,
		controller.NewInfo(ControllerID, Version, "browser@"+cc.GetName()),
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return NewVolume(ctx, le, cc)
		},
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
