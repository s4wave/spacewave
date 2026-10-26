package core_alarm_watch

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	alarm_watch "github.com/s4wave/spacewave/sdk/alarm/watch"
	function_run "github.com/s4wave/spacewave/sdk/function/run"
	"github.com/tetratelabs/wazero"
)

// Factory constructs Alarm watch controllers. Its watches share the native
// code compiled from the QuickJS reactor.
type Factory struct {
	// bus is the controller bus.
	bus bus.Bus
	// compiled caches the native code of the QuickJS reactor.
	compiled wazero.CompilationCache
}

// NewFactory builds an Alarm watch controller factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus, compiled: wazero.NewCompilationCache()}
}

// GetConfigID returns the unique ID for the config.
func (f *Factory) GetConfigID() string {
	return alarm_watch.ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &alarm_watch.Config{}
}

// Construct constructs the Alarm watch controller for a config.
func (f *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	return NewController(le, f.bus, conf.(*alarm_watch.Config), function_run.NewRunner(le, f.compiled, nil)), nil
}

// GetVersion returns the version of this controller.
func (f *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
