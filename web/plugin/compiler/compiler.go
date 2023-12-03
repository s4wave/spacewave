package bldr_web_plugin_compiler

import (
	"context"
	"path/filepath"

	random_id "github.com/aperturerobotics/bifrost/util/randstring"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	"github.com/aperturerobotics/util/fsutil"
	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/web/entrypoint/electron/bundle"
	web_plugin_controller "github.com/aperturerobotics/bldr/web/plugin/controller"
	electron "github.com/aperturerobotics/bldr/web/plugin/electron"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/blang/semver"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/plugin/compiler"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "web runtime plugin compiler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// basePkg is the base go module path for bldr.
const basePkg = "github.com/aperturerobotics/bldr"

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
			return &Controller{
				BusController: base,
			}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// BuildManifest attempts to compile the manifest once.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
) (*bldr_manifest_builder.BuilderResult, error) {
	pluginCompilerConf := plugin_compiler.NewConfig()
	pluginCompilerConf.GoPkgs = []string{
		basePkg + "/web/plugin/controller",
	}
	pluginCompilerConf.DisableFetchAssets = true
	pluginCompilerConf.DisableRpcFetch = true
	pluginCompilerConf.DelveAddr = c.GetConfig().GetDelveAddr()

	// configure running the web plugin controller
	webPluginCtrlConf, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &web_plugin_controller.Config{}),
		false,
	)
	if err != nil {
		return nil, err
	}

	// build config set for the plugin
	pluginCompilerConf.ConfigSet = map[string]*configset_proto.ControllerConfig{
		"web-plugin": webPluginCtrlConf,
	}

	pluginCompilerCtrl, err := plugin_compiler.NewController(c.GetLogger(), c.GetBus(), pluginCompilerConf)
	if err != nil {
		return nil, err
	}

	// bundle electron, if applicable.
	pluginCompilerCtrl.AddPreBuildHook(c.BundleElectronHook)

	// build the manifest
	return pluginCompilerCtrl.BuildManifest(ctx, args)
}

// GetElectronApplicable returns if electron should be bundled for this platform.
func GetElectronApplicable(parsedPlatform bldr_platform.Platform) bool {
	_, ok := parsedPlatform.(*bldr_platform.NativePlatform)
	return ok
}

// BundleElectronHook bundles electron for the platform ID, if applicable.
// If the platform ID is not applicable, returns nil.
func (c *Controller) BundleElectronHook(
	ctx context.Context,
	builderConf *manifest_builder.BuilderConfig,
	worldEng world.Engine,
) (*plugin_compiler.PreBuildHookResult, error) {
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}
	if !GetElectronApplicable(buildPlatform) {
		// TODO: build web plugin shim for web platform
		// TODO: return error if unrecognized platform id
		return nil, nil
	}

	platformID := meta.GetPlatformId()
	pluginID := meta.GetManifestId()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	minify, devMode := buildType.IsRelease(), buildType.IsDev()
	workingDir := filepath.Join(builderConf.GetWorkingPath(), "build")

	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building web plugin with plugin compiler")

	// clean / create electron assets dir
	outDistPath := filepath.Join(builderConf.GetWorkingPath(), "dist")
	electronDistPath := filepath.Join(outDistPath, "electron")
	if err := fsutil.CleanCreateDir(electronDistPath); err != nil {
		return nil, err
	}

	// download the electron redistributable with npm
	if err := entrypoint_electron_bundle.DownloadElectronRedist(
		ctx,
		le,
		buildPlatform,
		workingDir,
		electronDistPath,
		c.GetConfig().GetElectronPkg(),
	); err != nil {
		return nil, err
	}

	// create a dir for building the web entrypoint
	workingEntrypointDir := filepath.Join(workingDir, "web-entry")
	if err := fsutil.CleanCreateDir(workingEntrypointDir); err != nil {
		return nil, err
	}

	// build the electron entrypoint to the working entrypoint dir
	le.Debug("building electron entrypoint")
	entrypoint_electron_bundle.EsbuildLogLevel = esbuild.LogLevelError
	distSrcDir := builderConf.GetDistSourcePath()
	err = entrypoint_electron_bundle.BuildElectronBundle(ctx, le, distSrcDir, workingEntrypointDir, minify, devMode)
	if err != nil {
		return nil, err
	}

	// build the bundle asar
	distAsarPath := filepath.Join(outDistPath, "app.asar")
	if err := entrypoint_electron_bundle.BuildAsar(ctx, le, workingEntrypointDir, distAsarPath); err != nil {
		return nil, errors.Wrap(err, "build app.asar")
	}

	var extraElectronFlags []string
	if buildType.IsDev() {
		extraElectronFlags = append(extraElectronFlags, "--inspect=5858")
	}

	// build config set to start the electron entrypoint on startup
	webRuntimeId := random_id.RandomIdentifier(0)
	electronCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, &electron.Config{
		WebRuntimeId:  webRuntimeId,
		ElectronPath:  filepath.Join("electron", entrypoint_electron_bundle.GetElectronBinName(buildPlatform)),
		RendererPath:  "app.asar/index.mjs",
		ElectronFlags: extraElectronFlags,
	}), false)
	if err != nil {
		return nil, err
	}

	// return result
	return &plugin_compiler.PreBuildHookResult{
		Config: &plugin_compiler.Config{
			ConfigSet: map[string]*configset_proto.ControllerConfig{
				"electron": electronCtrlConf,
			},
			GoPkgs: []string{
				basePkg + "/web/plugin/electron",
			},
		},
	}, nil
}

// _ is a type assertion
var (
	_ plugin_compiler.PreBuildHook = ((*Controller)(nil)).BundleElectronHook
	_ manifest_builder.Controller  = ((*Controller)(nil))
)
