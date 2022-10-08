package plugin_compiler

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
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
	conf := c.GetConfig()
	pluginID := conf.GetPluginId()
	le := c.GetLogger().WithField("plugin-id", pluginID)

	le.Info("analyzing go packages")
	an, err := AnalyzePackages(ctx, le, conf.GetSourcePath(), conf.GetGoPackages())
	if err != nil {
		return err
	}

	cleanCreateDir := func(path string) error {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		return nil
	}

	// clean / create dist dir
	outDistPath := path.Join(conf.GetWorkingPath(), "dist")
	if err := cleanCreateDir(outDistPath); err != nil {
		return err
	}

	// clean / create web assets dir
	outWebPath := path.Join(conf.GetWorkingPath(), "web")
	if err := cleanCreateDir(outWebPath); err != nil {
		return err
	}

	// compile Go modules
	mc, err := NewModuleCompiler(ctx, le, conf.GetWorkingPath(), pluginID)
	if err != nil {
		return err
	}

	le.Info("generating go packages")
	if err := mc.GenerateModule(an); err != nil {
		return err
	}

	le.Info("compiling go packages")
	entrypointFilename := "entrypoint"
	outDistBinary := path.Join(outDistPath, entrypointFilename)
	if err := mc.CompilePlugin(outDistBinary); err != nil {
		return err
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), conf.GetEngineId())
	_ = busEngine

	// bundle dist directory
	le.Info("bundling plugin files")
	ts := timestamp.Now()
	distFs := os.DirFS(outDistPath)
	webAssetsFs := os.DirFS(outWebPath)
	manifestRef, err := world.AccessObject(ctx, busEngine.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return plugin.CreatePluginManifest(ctx, bcs, pluginID, entrypointFilename, distFs, webAssetsFs, &ts)
	})
	if err != nil {
		return err
	}
	le.Infof("bundled plugin files to manifest: %s", manifestRef.MarshalString())

	// TODO TODO
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
