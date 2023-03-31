package bldr_web_plugin_compiler

import (
	"context"
	"os"
	"path"

	manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	"github.com/aperturerobotics/bldr/platform"
	plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	"github.com/aperturerobotics/bldr/util/fsutil"
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
	builderConf *bldr_manifest_builder.BuilderConfig,
) (*bldr_manifest_builder.BuilderResult, error) {
	pluginCompilerConf := plugin_compiler.NewConfig()
	pluginCompilerConf.GoPackages = []string{
		basePkg + "/web/plugin/controller",
	}
	pluginCompilerConf.DisableFetchAssets = true
	pluginCompilerConf.DisableRpcFetch = true

	// build config set for the plugin
	webPluginCtrlConf, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &web_plugin_controller.Config{}),
		false,
	)
	if err != nil {
		return nil, err
	}
	pluginCompilerConf.ConfigSet = map[string]*configset_proto.ControllerConfig{
		"web-plugin": webPluginCtrlConf,
	}

	pluginCompilerCtrl, err := plugin_compiler.NewController(c.GetLogger(), c.GetBus(), pluginCompilerConf)
	if err != nil {
		return nil, err
	}

	// bundle electron, if applicable.
	pluginCompilerCtrl.AddPreBuildHook(c.BundleElectron)

	// build the manifest
	return pluginCompilerCtrl.BuildManifest(ctx, builderConf)
}

// GetElectronApplicable returns if electron should be bundled for this platform.
func GetElectronApplicable(parsedPlatform platform.Platform) bool {
	_, ok := parsedPlatform.(*platform.NativePlatform)
	return ok
}

// BundleElectron bundles electron for the platform ID, if applicable.
// If the platform ID is not applicable, returns nil.
func (c *Controller) BundleElectron(ctx context.Context, builderConf *manifest_builder.BuilderConfig, worldEng world.Engine) (*plugin_compiler.PreBuildHookResult, error) {
	meta := builderConf.GetManifestMeta()
	pluginID := meta.GetManifestId()
	sourcePath := builderConf.GetSourcePath()
	buildType := manifest.ToBuildType(meta.GetBuildType())
	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType)
	pluginPlatformID := meta.GetPlatformId()
	parsedPlatform, err := platform.ParsePlatform(pluginPlatformID)
	if err != nil {
		return nil, err
	}
	if !GetElectronApplicable(parsedPlatform) {
		return nil, nil
	}

	// find the path to the asar bundler
	nodeModulesPath := path.Join(sourcePath, "node_modules")
	nodeBinPath := path.Join(nodeModulesPath, ".bin")
	asarBinPath := path.Join(nodeBinPath, "asar")
	if _, err := os.Stat(asarBinPath); err != nil {
		err = errors.Wrap(err, "asar not in node_modules: install with npm i --dev @electron/asar")
		return nil, err
	}

	// Currently we just load the local native electron dist version.
	// need to translate the go compiler GOOS/GOARCH pair into a --target_arch for npm.
	// NOTE: we can use the world as a cache for the electron dists as well.
	// (use a common unixfs, unpack the electron files, trust unixfs to do deduplication)

	// find the path to electron
	// NOTE: in future we can use: npm i --target_arch=x64 --target_platform=linux
	// NOTE: alternatively: electron-build --windows
	electronSrcPath := path.Join(nodeModulesPath, "electron", "dist")
	if _, err := os.Stat(electronSrcPath); err != nil {
		err = errors.Wrap(err, "electron not in node_modules: install with npm i --dev electron")
		return nil, err
	}

	// clean / create intermediate electron assets dir
	outDistPath := path.Join(builderConf.GetWorkingPath(), "dist")
	electronDistPath := path.Join(outDistPath, "electron")
	if err := fsutil.CleanCreateDir(electronDistPath); err != nil {
		return nil, err
	}

	// create a dir for building the web entrypoint
	workingEntrypointDir := path.Join(builderConf.GetWorkingPath(), "build", "web-entry")
	if err := fsutil.CleanCreateDir(workingEntrypointDir); err != nil {
		return nil, err
	}

	// copy electron dist files to dist/
	le.Debug("copying electron dist files")
	if err := fsutil.CopyRecursive(electronDistPath, electronSrcPath, nil); err != nil {
		return nil, err
	}

	// build the electron entrypoint to the working entrypoint dir
	le.Debug("building electron entrypoint")
	entrypoint_electron_bundle.EsbuildLogLevel = esbuild.LogLevelError
	distSrcDir := builderConf.GetDistSourcePath()
	minify := manifest.BuildType(meta.GetBuildType()).IsRelease()
	err = entrypoint_electron_bundle.BuildBrowserBundle(le, distSrcDir, workingEntrypointDir, minify)
	if err != nil {
		return nil, err
	}

	// build the bundle asar
	distAsarPath := path.Join(outDistPath, "app.asar")
	if err := entrypoint_electron_bundle.BuildAsar(ctx, le, asarBinPath, workingEntrypointDir, distAsarPath); err != nil {
		return nil, errors.Wrap(err, "build app.asar")
	}

	// build config set to start the electron entrypoint on startup
	electronCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, &electron.Config{
		ElectronPath: "./electron",
		RendererPath: "./app.asar",
	}), false)
	if err != nil {
		return nil, err
	}
	configSet := map[string]*configset_proto.ControllerConfig{
		"electron": electronCtrlConf,
	}

	// return result
	return &plugin_compiler.PreBuildHookResult{
		ConfigSet: configSet,
		GoPackages: []string{
			basePkg + "/web/plugin/electron",
		},
	}, nil
}

// _ is a type assertion
var (
	_ plugin_compiler.PreBuildHook = ((*Controller)(nil)).BundleElectron
	_ manifest_builder.Controller  = ((*Controller)(nil))
)
