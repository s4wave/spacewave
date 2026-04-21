package plugin_host_web

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
)

// QuickJSConfigID is the config identifier for the QuickJS plugin host.
const QuickJSConfigID = WebQuickJSHostControllerID

// QuickJSFactory constructs a QuickJS plugin host.
type QuickJSFactory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewQuickJSFactory builds the factory.
func NewQuickJSFactory(bus bus.Bus) *QuickJSFactory {
	return &QuickJSFactory{bus: bus}
}

// GetConfigID returns the configuration ID for the controller.
func (t *QuickJSFactory) GetConfigID() string {
	return QuickJSConfigID
}

// GetControllerID returns the unique ID for the controller.
func (t *QuickJSFactory) GetControllerID() string {
	return WebQuickJSHostControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *QuickJSFactory) ConstructConfig() config.Config {
	return &QuickJSConfig{}
}

// Construct constructs the associated controller given configuration.
func (t *QuickJSFactory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*QuickJSConfig)

	hostCtrl, _, err := NewWebQuickJSHostController(le, t.bus, cc)
	if err != nil {
		return nil, err
	}
	return hostCtrl, nil
}

// GetVersion returns the version of this controller.
func (t *QuickJSFactory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*QuickJSFactory)(nil))
