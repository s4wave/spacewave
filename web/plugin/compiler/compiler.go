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
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin_compiler_go "github.com/aperturerobotics/bldr/plugin/compiler/go"
	"github.com/aperturerobotics/bldr/util/npm"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/web/entrypoint/electron/bundle"
	entrypoint_saucer_bundle "github.com/aperturerobotics/bldr/web/entrypoint/saucer/bundle"
	web_plugin_browser_build "github.com/aperturerobotics/bldr/web/plugin/browser/build"
	electron "github.com/aperturerobotics/bldr/web/plugin/electron"
	saucer "github.com/aperturerobotics/bldr/web/plugin/saucer"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
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
	if bldr_platform.IsWebPlatform(buildPlatform) {
		return c.buildBrowserShimManifest(ctx, args)
	}

	pluginCompilerCtrl, err := plugin_compiler_go.NewController(c.GetLogger(), c.GetBus(), pluginCompilerConf)
	if err != nil {
		return nil, err
	}

	// Check which web renderer to bundle based on BLDR_WEB_RENDERER env var.
	renderer := web_runtime.GetWebRendererFromEnv().Resolve()
	switch renderer {
	case web_runtime.WebRenderer_WEB_RENDERER_SAUCER:
		pluginCompilerCtrl.AddPreBuildHook(c.BundleSaucerHook)
	case web_runtime.WebRenderer_WEB_RENDERER_ELECTRON:
		pluginCompilerCtrl.AddPreBuildHook(c.BundleElectronHook)
	}

	// build the manifest
	return pluginCompilerCtrl.BuildManifest(ctx, args, host)
}

// BundleElectronHook bundles electron.
func (c *Controller) BundleElectronHook(
	ctx context.Context,
	builderConf *bldr_manifest_builder.BuilderConfig,
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

	// If this is a wasm platform (native/js/wasm), do not bundle electron.
	// Electron is not available for wasm targets.
	if bldr_platform.IsWebPlatform(buildPlatform) {
		return nil, nil
	}

	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
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

	// download the electron redistributable with bun
	stateDir := builderConf.GetWorkingPath()
	if err := entrypoint_electron_bundle.DownloadElectronRedist(
		ctx,
		le,
		stateDir,
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
	err = entrypoint_electron_bundle.BuildElectronBundle(ctx, le, stateDir, distSrcDir, workingEntrypointDir, webStartupSrcPath, minify, devMode)
	if err != nil {
		return nil, err
	}

	// build the bundle asar
	distAsarPath := filepath.Join(outDistPath, "app.asar")
	if err := entrypoint_electron_bundle.BuildAsar(ctx, le, stateDir, workingEntrypointDir, distAsarPath); err != nil {
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
	electronConf := &electron.Config{
		WebRuntimeId:  webRuntimeId,
		ElectronPath:  filepath.Join("electron", entrypoint_electron_bundle.GetElectronBinName(buildPlatform)),
		RendererPath:  "app.asar/index.mjs",
		ElectronFlags: extraElectronFlags,
	}

	// Copy native app branding from compiler config to electron config.
	if nativeApp := c.GetConfig().GetNativeApp(); nativeApp != nil {
		electronConf.AppName = nativeApp.GetAppName()
		electronConf.WindowTitle = nativeApp.GetWindowTitle()
		electronConf.WindowWidth = nativeApp.GetWindowWidth()
		electronConf.WindowHeight = nativeApp.GetWindowHeight()
		electronConf.DevTools = nativeApp.GetDevTools()
		electronConf.ThemeSource = nativeApp.GetThemeSource()
	}

	electronCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, electronConf), false)
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

// BundleSaucerHook bundles saucer.
func (c *Controller) BundleSaucerHook(
	ctx context.Context,
	builderConf *bldr_manifest_builder.BuilderConfig,
	worldEng world.Engine,
) (*plugin_compiler_go.PreBuildHookResult, error) {
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// If this is not the native platform, do not bundle saucer.
	if buildPlatform.GetBasePlatformID() != bldr_platform.PlatformID_NATIVE {
		return nil, nil
	}

	// If this is a wasm platform (native/js/wasm), do not bundle saucer.
	if bldr_platform.IsWebPlatform(buildPlatform) {
		return nil, nil
	}

	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	platformID := meta.GetPlatformId()
	pluginID := meta.GetManifestId()
	minify := buildType.IsRelease()

	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building web plugin with saucer")

	outDistPath := filepath.Join(builderConf.GetWorkingPath(), "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}

	// Build directories for saucer resolution
	stateDir := builderConf.GetWorkingPath()
	buildDir := filepath.Join(stateDir, "build", "saucer")
	binDir := filepath.Join(outDistPath, "bin")
	for _, dir := range []string{buildDir, binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	// BLDR_FROM_SOURCE=1 forces building saucer from vendored C++ sources.
	if os.Getenv("BLDR_FROM_SOURCE") != "" {
		le.Info("building saucer from source (BLDR_FROM_SOURCE set)")
		vendorDir := filepath.Join(builderConf.GetSourcePath(), "vendor")
		saucerBuildDir := filepath.Join(buildDir, "bldr-saucer-build")
		if err := entrypoint_saucer_bundle.BuildSaucerFromSource(ctx, le, vendorDir, saucerBuildDir, binDir, buildPlatform); err != nil {
			return nil, errors.Wrap(err, "build saucer from source")
		}
	} else {
		// Resolve the saucer binary from the @aptre/bldr-saucer npm package.
		saucerPkg := "@aptre/bldr-saucer@latest"
		packageJsonPath := filepath.Join(builderConf.GetSourcePath(), "package.json")
		saucerVer, pkgErr := npm.LoadPackageVersion(packageJsonPath, "@aptre/bldr-saucer")
		if pkgErr != nil {
			le.WithError(pkgErr).Warn("unable to load package.json to determine saucer version")
			saucerVer = ""
		}
		if saucerVer != "" {
			saucerPkg = "@aptre/bldr-saucer@" + strings.TrimSpace(saucerVer)
		}

		// Cache resolved binaries to avoid re-downloading across incremental builds.
		cacheDir := filepath.Join(stateDir, "cache", "saucer")

		le.Info("resolving saucer binary...")
		if err := entrypoint_saucer_bundle.ResolveSaucerBinary(ctx, le, stateDir, binDir, cacheDir, buildPlatform, saucerPkg); err != nil {
			return nil, errors.Wrap(err, "resolve saucer binary")
		}
	}
	le.Info("saucer binary resolved successfully")

	// Build JS bundle
	le.Debug("building saucer JS bundle...")
	distSrcDir := builderConf.GetDistSourcePath()
	jsBundle, err := entrypoint_saucer_bundle.BuildSaucerJSBundle(le, distSrcDir, buildDir, minify)
	if err != nil {
		return nil, errors.Wrap(err, "build saucer JS bundle")
	}

	// Build config set to start the saucer entrypoint on startup
	webRuntimeId := random_id.RandomIdentifier(0)
	saucerConf := &saucer.Config{
		WebRuntimeId:  webRuntimeId,
		SaucerPath:    filepath.Join("bin", entrypoint_saucer_bundle.GetSaucerBinName(buildPlatform)),
		DevTools:      buildType.IsDev(),
		BootstrapHtml: jsBundle.BootstrapHTML,
		EntrypointJs:  jsBundle.EntrypointJS,
	}

	// Copy native app branding from compiler config to saucer config.
	if nativeApp := c.GetConfig().GetNativeApp(); nativeApp != nil {
		saucerConf.AppName = nativeApp.GetAppName()
		saucerConf.WindowTitle = nativeApp.GetWindowTitle()
		saucerConf.WindowWidth = nativeApp.GetWindowWidth()
		saucerConf.WindowHeight = nativeApp.GetWindowHeight()
		if nativeApp.GetDevTools() {
			saucerConf.DevTools = true
		}
	}

	saucerCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, saucerConf), false)
	if err != nil {
		return nil, err
	}

	// return result
	return &plugin_compiler_go.PreBuildHookResult{
		Config: &plugin_compiler_go.Config{
			ConfigSet: map[string]*configset_proto.ControllerConfig{
				"saucer": saucerCtrlConf,
			},
			GoPkgs: []string{
				basePkg + "/web/plugin/saucer",
			},
			WebPkgs: bldr_web_bundler.GetBldrDistWebPkgRefConfigs(),
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
	result := bldr_manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		nil,
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// GetSupportedPlatforms returns the base platform IDs this compiler supports.
func (c *Controller) GetSupportedPlatforms() []string {
	return []string{bldr_platform.PlatformID_NATIVE}
}

// _ is a type assertion
var (
	_ plugin_compiler_go.PreBuildHook  = ((*Controller)(nil)).BundleElectronHook
	_ plugin_compiler_go.PreBuildHook  = ((*Controller)(nil)).BundleSaucerHook
	_ bldr_manifest_builder.Controller = ((*Controller)(nil))
)
