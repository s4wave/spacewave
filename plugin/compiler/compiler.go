package plugin_compiler

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the compiler controller ID.
const ControllerID = "bldr/plugin/compiler"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "plugin compiler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
}

// NewController constructs a new compiler controller.
func NewController(le *logrus.Entry, b bus.Bus, cc *Config) *Controller {
	return &Controller{
		BusController: bus.NewBusController(
			le,
			b,
			cc,
			ControllerID,
			Version,
			controllerDescrip,
		),
	}
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewFactory constructs a new plugin compiler controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		NewConfig,
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	le, conf := c.GetLogger(), c.GetConfig()
	pluginID := conf.GetPluginId()
	_ = pluginID

	le.Info("analyzing go packages")
	an, err := AnalyzePackages(ctx, le, conf.GetSourcePath(), conf.GetGoPackages())
	if err != nil {
		return err
	}
	_ = an

	outDistPath := path.Join(conf.GetWorkingPath(), "dist")
	if err := os.MkdirAll(outDistPath, 0755); err != nil {
		return err
	}

	mc, err := NewModuleCompiler(ctx, le, conf.GetWorkingPath(), pluginID)
	if err != nil {
		return err
	}

	le.Info("generating go packages")
	if err := mc.GenerateModule(an); err != nil {
		return err
	}

	le.Info("compiling go packages")
	outDistBinary := path.Join(outDistPath, "entrypoint")
	if err := mc.CompilePlugin(outDistBinary); err != nil {
		return err
	}

	// TODO TODO
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
