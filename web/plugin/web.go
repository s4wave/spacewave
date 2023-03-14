package plugin_web

import (
	"context"
	"os"
	"path"

	manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	plugin_platform "github.com/aperturerobotics/bldr/plugin/platform"
	"github.com/aperturerobotics/bldr/util/fsutil"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/web/entrypoint/electron/bundle"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/promise"
	"github.com/blang/semver"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/plugin"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "web runtime plugin builder controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
	resultPromise *promise.PromiseContainer[*manifest_builder.BuilderResult]
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
			return &Controller{
				BusController: base,
				resultPromise: promise.NewPromiseContainer[*manifest_builder.BuilderResult](),
			}, nil
		},
	)
}

// GetResultPromise returns the plugin result promise.
// Also contains any error that occurs while compiling.
func (c *Controller) GetResultPromise() *promise.PromiseContainer[*manifest_builder.BuilderResult] {
	return c.resultPromise
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	conf := c.GetConfig()
	builderConf := conf.GetBuilderConfig()
	meta := builderConf.GetManifestMeta()
	pluginID := meta.GetManifestId()
	sourcePath := builderConf.GetSourcePath()
	buildType := manifest.ToBuildType(meta.GetBuildType())
	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType)

	// determine the strategy: currently only Electron is supported
	pluginPlatformID := meta.GetPlatformId()
	if pluginPlatformID != plugin_platform.PlatformID_NATIVE {
		err := errors.Errorf("web: not needed / not supported for plugin platform: %s", pluginPlatformID)
		le.Debug(err.Error())
		c.resultPromise.SetResult(nil, err)
		return nil // return nil error
	}

	// find the path to the asar bundler
	nodeModulesPath := path.Join(sourcePath, "node_modules")
	nodeBinPath := path.Join(nodeModulesPath, ".bin")
	asarBinPath := path.Join(nodeBinPath, "asar")
	if _, err := os.Stat(asarBinPath); err != nil {
		err = errors.Wrap(err, "asar not in node_modules: install with npm i --dev @electron/asar")
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// find the path to electron
	// NOTE: in future we can use: npm i --target_arch=x64 --target_platform=linux
	// NOTE: alternatively: electron-build --windows
	electronSrcPath := path.Join(nodeModulesPath, "electron", "dist")
	if _, err := os.Stat(electronSrcPath); err != nil {
		err = errors.Wrap(err, "electron not in node_modules: install with npm i --dev electron")
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// clean / create dist dir
	outDistPath := path.Join(builderConf.GetWorkingPath(), "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// clean / create assets dir
	outAssetsPath := path.Join(builderConf.GetWorkingPath(), "assets")
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// clean / create intermediate electron assets dir
	workingEntrypointDir := path.Join(builderConf.GetWorkingPath(), "build", "entry")
	if err := fsutil.CleanCreateDir(workingEntrypointDir); err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// copy electron dist files to dist/
	le.Debug("copying electron dist files")
	if err := fsutil.CopyRecursive(outDistPath, electronSrcPath, nil); err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// build the electron entrypoint to the working entrypoint dir
	le.Debug("building electron entrypoint")
	entrypoint_electron_bundle.EsbuildLogLevel = esbuild.LogLevelError
	distSrcDir := builderConf.GetDistSourcePath()
	minify := manifest.BuildType(meta.GetBuildType()).IsRelease()
	err := entrypoint_electron_bundle.BuildBrowserBundle(le, distSrcDir, workingEntrypointDir, minify)
	if err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// build the bundle asar
	distAsarPath := path.Join(outDistPath, "app.asar")
	if err := entrypoint_electron_bundle.BuildAsar(ctx, le, asarBinPath, workingEntrypointDir, distAsarPath); err != nil {
		err = errors.Wrap(err, "build app.asar")
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())
	defer busEngine.Close()

	// bundle the plugin entrypoint
	entrypointBinName := "entrypoint"
	entrypointBinPath := path.Join(outDistPath, entrypointBinName)
	if err := compilePluginEntrypoint(le, distSrcDir, entrypointBinPath); err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// bundle the plugin manifest
	distFs, assetsFs := os.DirFS(outDistPath), os.DirFS(outAssetsPath)
	committedManifest, committedManifestRef, err := builderConf.CommitManifest(
		ctx,
		le,
		busEngine,
		meta,
		entrypointBinName,
		distFs,
		assetsFs,
	)
	if err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	le.Info("successfully bundled electron to web plugin")
	c.resultPromise.SetResult(manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
	), nil)
	return nil
}

// compilePluginEntrypoint compiles the plugin entrypoint to outFile.
func compilePluginEntrypoint(le *logrus.Entry, distSrcPath, outFile string) error {
	args := []string{
		"build",
		"-v", "-trimpath",
		"-buildvcs=false",
		"-o",
		outFile,
	}

	// build path
	args = append(args, "github.com/aperturerobotics/bldr/web/plugin/electron/entrypoint")

	// go build
	ecmd := gocompiler.NewGoCompilerCmd(args...)
	ecmd.Dir = distSrcPath
	return gocompiler.ExecGoCompiler(le, ecmd)
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
