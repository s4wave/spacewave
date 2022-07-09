package electron

import (
	"context"
	"os"
	"path"

	storage "github.com/aperturerobotics/bldr/storage/desktop"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	rc "github.com/aperturerobotics/bldr/web/runtime/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Factory constructs a Electron runtime controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a Browser runtime factory.
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
	le := opts.GetLogger()
	cc := conf.(*Config)

	storagePath := cc.GetStoragePath()
	if storagePath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		storagePath = path.Join(configDir, "aperture_robotics")
	}

	webRuntimeId := cc.GetWebRuntimeId()
	if webRuntimeId == "" {
		webRuntimeId = "default"
	}

	// Construct the runtime controller.
	return rc.NewController(
		le,
		t.bus,
		func(
			ctx context.Context,
			le *logrus.Entry,
			handler web_runtime.WebRuntimeHandler,
		) (web_runtime.WebRuntime, error) {
			st := storage.BuildStorage(t.bus, storagePath)
			return NewRuntime(
				le,
				t.bus,
				handler,
				st,
				cc.GetElectronPath(),
				cc.GetRendererPath(),
				webRuntimeId,
			)
		},
		RuntimeID,
		Version,
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
