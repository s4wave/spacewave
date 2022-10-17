package plugin_compiler

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	web_fetch_controller "github.com/aperturerobotics/bldr/web/fetch/service"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
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
	builderConf := conf.GetPluginBuilderConfig()
	pluginID := builderConf.GetPluginId()
	sourcePath := builderConf.GetSourcePath()
	le := c.GetLogger().WithField("plugin-id", pluginID)

	le.Info("checking module file")
	err := MaybeRunGoModTidy(ctx, le, sourcePath)
	if err != nil {
		return err
	}

	le.Info("analyzing go packages")
	goPkgs := conf.GetGoPackages()
	an, err := AnalyzePackages(ctx, le, sourcePath, goPkgs)
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
	outDistPath := path.Join(builderConf.GetWorkingPath(), "dist")
	if err := cleanCreateDir(outDistPath); err != nil {
		return err
	}

	// clean / create web assets dir
	outWebPath := path.Join(builderConf.GetWorkingPath(), "web")
	if err := cleanCreateDir(outWebPath); err != nil {
		return err
	}

	// compile Go modules
	mc, err := NewModuleCompiler(ctx, le, builderConf.GetWorkingPath(), pluginID)
	if err != nil {
		return err
	}

	// build the config set based on configuration
	embedConfigSet := make(configset_proto.ConfigSetMap)
	if !conf.GetDisableRpcFetch() {
		embedConfigSet["rpc-fetch"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, web_fetch_controller.NewConfig()),
		)
		if err != nil {
			return err
		}
	}
	if !conf.GetDisableFetchAssets() {
		embedConfigSet["plugin-assets"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, plugin_assets_http.NewConfig("", "")),
		)
		if err != nil {
			return err
		}
	}

	// merge configured config set entries
	configset_proto.MergeConfigSetMaps(embedConfigSet, conf.GetConfigSet())

	// encode config set for embedded config set binary
	var configSetBin []byte
	if len(embedConfigSet) != 0 {
		configSetObj := &configset_proto.ConfigSet{
			Configurations: conf.GetConfigSet(),
		}
		configSetBin, err = configSetObj.MarshalVT()
		if err != nil {
			return err
		}
	}

	le.Info("generating go packages")
	if err := mc.GenerateModule(an, configSetBin); err != nil {
		return err
	}

	le.Info("compiling go packages")
	entrypointFilename := "entrypoint"
	outDistBinary := path.Join(outDistPath, entrypointFilename)
	if err := mc.CompilePlugin(outDistBinary); err != nil {
		return err
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), conf.GetPluginBuilderConfig().GetEngineId())
	defer busEngine.Close()

	// bundle dist directory
	le.Debug("bundling plugin files")
	ts := timestamp.Now()
	distFs := os.DirFS(outDistPath)
	webAssetsFs := os.DirFS(outWebPath)
	manifestRef, err := world.AccessObject(ctx, busEngine.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return plugin.CreatePluginManifest(ctx, bcs, pluginID, entrypointFilename, distFs, webAssetsFs, &ts)
	})
	if err != nil {
		return err
	}

	// push to the plugin host world
	le.Infof("committing plugin manifest to world: %s", manifestRef.MarshalString())
	tx, err := busEngine.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	opPeerID, err := conf.GetPluginBuilderConfig().ParsePeerID()
	if err != nil {
		return err
	}

	_, _, err = tx.ApplyWorldOp(
		plugin_host.NewUpdatePluginManifestOp(
			conf.GetPluginBuilderConfig().GetPluginHostKey(),
			pluginID,
			manifestRef,
		),
		opPeerID,
	)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	le.Info("plugin build complete")

	// TODO TODO
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
