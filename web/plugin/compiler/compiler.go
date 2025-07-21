//go:build !js

package bldr_web_plugin_compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	random_id "github.com/aperturerobotics/bifrost/util/randstring"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin_compiler_go "github.com/aperturerobotics/bldr/plugin/compiler/go"
	"github.com/aperturerobotics/bldr/util/npm"
	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/web/entrypoint/electron/bundle"
	web_plugin_browser_build "github.com/aperturerobotics/bldr/web/plugin/browser/build"
	electron "github.com/aperturerobotics/bldr/web/plugin/electron"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
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
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	pluginCompilerConf, err := c.GetConfig().ToPluginCompilerConf()
	if err != nil {
		return nil, err
	}

	_, buildPlatform, err := args.GetBuilderConfig().GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// Do nothing if we are not targeting a supported platform.
	//
	// TODO: how do we definitively know that the plugin host will run the web plugin?
	if _, ok := buildPlatform.(*bldr_platform.NativePlatform); !ok {
		c.GetLogger().Warnf("skipping build for non-go platform: %v", buildPlatform.GetInputPlatformID())
		return nil, nil
	}

	// When targeting native/js/wasm or js we assume that we are on the web platform.
	//
	// The web platform has the WebRuntime running on the plugin host. This is
	// because the web browser is responsible for loading the app and as such
	// the WebRuntime must already be running before starting plugins.
	//
	// Instead of bundling an entire Go program for the plugin in this case, we
	// can instead include a small .mjs shim which will load the desired config
	// sets to the host plugin bus via the WebRuntimeClient.
	//
	// TODO: how do we definitively know that the plugin host will run the web plugin?
	if buildPlatform.GetExecutableExt() == ".mjs" {
		return c.buildBrowserShimManifest(ctx, args)
	}

	pluginCompilerCtrl, err := plugin_compiler_go.NewController(c.GetLogger(), c.GetBus(), pluginCompilerConf)
	if err != nil {
		return nil, err
	}

	// bundle electron, if applicable.
	pluginCompilerCtrl.AddPreBuildHook(c.BundleElectronHook)

	// build the manifest
	return pluginCompilerCtrl.BuildManifest(ctx, args, host)
}

// BundleElectronHook bundles electron.
func (c *Controller) BundleElectronHook(
	ctx context.Context,
	builderConf *manifest_builder.BuilderConfig,
	worldEng world.Engine,
) (*plugin_compiler_go.PreBuildHookResult, error) {
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// If this is not the native platform, do not bundle electron.
	if buildPlatform.GetBasePlatformID() != bldr_platform.PlatformID_NATIVE {
		return nil, nil
	}

	// HACK: This is used by start-web-ws to disable bundling electron.
	// HACK: Replace this with something better.
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	if buildType.IsDev() && os.Getenv("BLDR_PLUGIN_WEB_SKIP_ELECTRON") == "true" {
		c.GetLogger().Debug("skipping bundling electron as the skip env var is set")
		return nil, nil
	}

	platformID := meta.GetPlatformId()
	pluginID := meta.GetManifestId()
	minify, devMode := buildType.IsRelease(), buildType.IsDev()
	workingDir := filepath.Join(builderConf.GetWorkingPath(), "build")

	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building web plugin")

	// clean / create electron assets dir
	outDistPath := filepath.Join(builderConf.GetWorkingPath(), "dist")
	electronDistPath := filepath.Join(outDistPath, "electron")
	if err := fsutil.CleanCreateDir(electronDistPath); err != nil {
		return nil, err
	}

	electronPkg := c.GetConfig().GetElectronPkg()
	if electronPkg == "" {
		// attempt to load version from package.json
		packageJsonPath := filepath.Join(builderConf.GetSourcePath(), "package.json")
		electronVer, err := npm.LoadPackageVersion(packageJsonPath, "electron")
		if err != nil {
			le.WithError(err).Warn("unable to load package.json to determine electron version")
			electronVer = ""
		}
		if electronVer == "" {
			electronVer = "latest"
		}
		electronVer = strings.TrimSpace(electronVer)
		electronPkg = "electron@" + electronVer
	}

	// download the electron redistributable with npm
	if err := entrypoint_electron_bundle.DownloadElectronRedist(
		ctx,
		le,
		buildPlatform,
		workingDir,
		electronDistPath,
		electronPkg,
	); err != nil {
		return nil, err
	}

	// create a dir for building the web entrypoint
	workingEntrypointDir := filepath.Join(workingDir, "web-entry")
	if err := fsutil.CleanCreateDir(workingEntrypointDir); err != nil {
		return nil, err
	}

	// TODO: set webStartupSrcPath to control the root component in the WebView.
	var webStartupSrcPath string

	// build the electron entrypoint to the working entrypoint dir
	le.Debug("building electron entrypoint")
	entrypoint_electron_bundle.EsbuildLogLevel = esbuild.LogLevelError
	distSrcDir := builderConf.GetDistSourcePath()
	err = entrypoint_electron_bundle.BuildElectronBundle(ctx, le, distSrcDir, workingEntrypointDir, webStartupSrcPath, minify, devMode)
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
		extraElectronFlags = append(
			extraElectronFlags,
			"--inspect=5858",
		)
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
	return &plugin_compiler_go.PreBuildHookResult{
		Config: &plugin_compiler_go.Config{
			ConfigSet: map[string]*configset_proto.ControllerConfig{
				"electron": electronCtrlConf,
			},
			GoPkgs: []string{
				basePkg + "/web/plugin/electron",
			},
		},
	}, nil
}

// buildBrowserShimManifest attempts to compile the web browser shim manifest once.
//
// TODO: replace the below code with a call to the js plugin compiler
func (c *Controller) buildBrowserShimManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
) (*bldr_manifest_builder.BuilderResult, error) {
	le := c.GetLogger()
	builderConf := args.GetBuilderConfig()
	meta, _, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	outDistPath := filepath.Join(builderConf.GetWorkingPath(), "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}

	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	isRelease := buildType.IsRelease()

	outFilename := "web.mjs"
	outFile := filepath.Join(outDistPath, outFilename)
	err = web_plugin_browser_build.BuildWebPluginBrowserEntrypoint(
		ctx,
		le,
		builderConf.GetDistSourcePath(),
		outFile,
		isRelease,
	)
	if err != nil {
		return nil, err
	}

	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())
	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	le.Debug("bundling plugin files")
	// bundle dist and assets fs
	committedManifest, committedManifestRef, err := builderConf.CommitManifestWithPaths(
		ctx,
		le,
		tx,
		meta,
		outFilename,
		outDistPath,
		"",
	)
	if err != nil {
		return nil, err
	}

	le.Debug("plugin build complete")
	result := manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		nil,
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// _ is a type assertion
var (
	_ plugin_compiler_go.PreBuildHook = ((*Controller)(nil)).BundleElectronHook
	_ manifest_builder.Controller     = ((*Controller)(nil))
)
