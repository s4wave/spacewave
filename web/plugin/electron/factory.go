package electron

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
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
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	webRuntimeId := cc.GetWebRuntimeId()
	if webRuntimeId == "" {
		webRuntimeId = "default"
	}

	workdirPath := cc.GetWorkdirPath()
	if workdirPath == "" {
		var err error
		workdirPath, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	// Construct the Electron controller.
	return NewController(
		le,
		t.bus,
		cc.GetElectronPath(),
		workdirPath,
		cc.GetRendererPath(),
		webRuntimeId,
		cc.GetElectronFlags(),
		&ElectronInit{
			ExternalLinks: cc.GetExternalLinks(),
			AppName:       cc.GetAppName(),
			WindowTitle:   cc.GetWindowTitle(),
			WindowWidth:   cc.GetWindowWidth(),
			WindowHeight:  cc.GetWindowHeight(),
			DevTools:      cc.GetDevTools(),
			ThemeSource:   cc.GetThemeSource(),
		},
	)
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
