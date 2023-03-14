package bldr_plugin_compiler

import (
	"context"
	"os"
	"path"
	"sort"
	"time"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host_configset "github.com/aperturerobotics/bldr/plugin/host/configset"
	"github.com/aperturerobotics/bldr/util/fsutil"
	web_fetch_controller "github.com/aperturerobotics/bldr/web/fetch/service"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/aperturerobotics/util/promise"
	"github.com/blang/semver"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
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
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType)

	// clean / create dist dir
	outDistPath := path.Join(builderConf.GetWorkingPath(), "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return err
	}

	// clean / create assets dir
	outAssetsPath := path.Join(builderConf.GetWorkingPath(), "assets")
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		return err
	}

	le.Info("checking module file")
	err := MaybeRunGoModTidy(ctx, le, sourcePath)
	if err != nil {
		return err
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), conf.GetBuilderConfig().GetEngineId())
	defer busEngine.Close()

	// Watcher
	var watcher *fsnotify.Watcher
	if !conf.GetDisableWatch() {
		watcher, err = fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		defer watcher.Close()
	}

	// Generate & build Go packages
	goPkgs := conf.GetGoPackages()
	watchedFiles := make(map[string]struct{})
	for {
		le.Debug("compiling plugin")
		entrypointFilename := "entrypoint"
		resultPromise := promise.NewPromise[*manifest_builder.BuilderResult]()
		c.resultPromise.SetPromise(resultPromise)

		// apply host config set
		configSet := make(map[string]*configset_proto.ControllerConfig, len(conf.GetConfigSet()))
		if len(conf.GetHostConfigSet()) != 0 {
			hostConfigSetConf, err := jsonpb.Marshal(&plugin_host_configset.Config{
				ConfigSet: conf.GetHostConfigSet(),
			})
			if err != nil {
				if err != context.Canceled {
					return err
				}
				err = errors.Wrap(err, "marshal plugin host configset")
				resultPromise.SetResult(nil, err)
				return err
			}

			configSet["plugin-host-configset"] = &configset_proto.ControllerConfig{
				Id:       plugin_host_configset.ConfigID,
				Revision: 1,
				Config:   hostConfigSetConf,
			}
		}
		for k, v := range conf.GetConfigSet() {
			configSet[k] = v.CloneVT()
		}

		_, consumedSrcFiles, err := c.BuildPlugin(
			ctx,
			le,
			pluginID,
			buildType,
			entrypointFilename,
			builderConf.GetWorkingPath(),
			sourcePath,
			outDistPath,
			outAssetsPath,
			goPkgs,
			conf.GetDisableRpcFetch(),
			conf.GetDisableFetchAssets(),
			conf.GetDelveAddr(),
			configSet,
		)
		if err != nil {
			if err == context.Canceled {
				return err
			}
			resultPromise.SetResult(nil, err)
			return err
		}

		le.Debug("bundling plugin files")
		// bundle dist and assets fs
		distFs, assetsFs := os.DirFS(outDistPath), os.DirFS(outAssetsPath)
		committedManifest, committedManifestRef, err := builderConf.CommitManifest(
			ctx,
			le,
			busEngine,
			meta,
			entrypointFilename,
			distFs,
			assetsFs,
		)
		if err != nil {
			resultPromise.SetResult(nil, err)
			return err
		}

		le.Info("plugin build complete")
		resultPromise.SetResult(manifest_builder.NewBuilderResult(
			committedManifest,
			committedManifestRef,
		), nil)
		if conf.GetDisableWatch() {
			le.Debug("disable_watch is set: returning after successful build")
			return nil
		}

		// build file watchlist
		nextWatchedFiles := make(map[string]struct{})
		for _, filePath := range consumedSrcFiles {
			nextWatchedFiles[filePath] = struct{}{}
		}

		// compare list of files with previous list of file
		for filePath := range watchedFiles {
			if _, ok := nextWatchedFiles[filePath]; ok {
				delete(nextWatchedFiles, filePath)
				continue
			}
			if watcher != nil {
				le.Debugf("removing watcher for file: %s", filePath)
				if err := watcher.Remove(filePath); err != nil {
					return err
				}
			}
		}
		for filePath := range nextWatchedFiles {
			watchedFiles[filePath] = struct{}{}
			if watcher != nil {
				if err := watcher.Add(filePath); err != nil {
					return err
				}
			}
		}

		if watcher == nil {
			le.Debugf(
				"built %d packages with %d files: watcher disabled, returning",
				len(goPkgs),
				len(watchedFiles),
			)
			return nil
		}

		// wait for a file change
		le.Debugf(
			"hot: watching %d packages with %d files",
			len(goPkgs),
			len(watchedFiles),
		)
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			ctx,
			watcher,
			time.Millisecond*500,
		)
		if err != nil {
			return err
		}

		le.Infof("re-analyzing packages after %d filesystem events", len(happened))
	}
}

// BuildPlugin compiles the plugin once, committing it to the target world.
//
// Returns a list of source files from the list of given goPkgs.
// Source files list includes all files consumed by esbuild.
// Called by Execute.
func (c *Controller) BuildPlugin(
	ctx context.Context,
	le *logrus.Entry,
	pluginID string,
	buildType bldr_manifest.BuildType,
	entrypointFilename,
	workingPath,
	sourcePath,
	outDistPath,
	outAssetsPath string,
	goPkgs []string,
	disableRpcFetch, disableFetchAssets bool,
	delveAddr string,
	configSet map[string]*configset_proto.ControllerConfig,
) (*Analysis, []string, error) {
	// build the config set based on configuration
	embedConfigSet := make(configset_proto.ConfigSetMap)
	var err error
	if !disableRpcFetch {
		embedConfigSet["rpc-fetch"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, web_fetch_controller.NewConfig()),
		)
		if err != nil {
			return nil, nil, err
		}
	}
	if !disableFetchAssets {
		embedConfigSet["plugin-assets"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, plugin_assets_http.NewConfig(plugin.PluginAssetsHttpPrefix, "")),
		)
		if err != nil {
			return nil, nil, err
		}
	}

	// merge configured config set entries
	configset_proto.MergeConfigSetMaps(embedConfigSet, configSet)

	// encode config set for embedded config set binary
	var configSetBin []byte
	if len(embedConfigSet) != 0 {
		configSetObj := &configset_proto.ConfigSet{
			Configurations: embedConfigSet,
		}
		configSetBin, err = configSetObj.MarshalVT()
		if err != nil {
			return nil, nil, err
		}
	}

	le.Info("analyzing go packages")
	an, err := AnalyzePackages(ctx, le, sourcePath, goPkgs)
	if err != nil {
		return nil, nil, err
	}

	// mapping between go.package.path.Variable and value
	// for the Go compiler linker flags
	isRelease := buildType.IsRelease()
	var goVariableDefs []*GoVarDef

	codeFiles := an.GetGoCodeFiles()
	fset := an.GetFileSet()

	// build source files list with go files
	var sourceFilesList []string
	for _, pkgFiles := range codeFiles {
		for _, codeFile := range pkgFiles {
			pkgFile := an.GetFileToken(codeFile)
			sourceFilesList = append(sourceFilesList, pkgFile.Name())
		}
	}

	// parse bldr:asset comments
	assetPkgs, err := an.FindAssetVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	if len(assetPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(assetPkgs), AssetTag)
		assetVarDefs, assetSrcPaths, err := BuildDefAssets(le, codeFiles, fset, assetPkgs, outAssetsPath, pluginID, isRelease)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, assetVarDefs...)
		sourceFilesList = append(sourceFilesList, assetSrcPaths...)
	}

	// parse bldr:asset:href comments
	assetHrefPkgs, err := an.FindAssetHrefVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	if len(assetHrefPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(assetHrefPkgs), AssetHrefTag)
		assetHrefDefs, err := BuildDefAssetHrefs(le, codeFiles, fset, assetHrefPkgs, outAssetsPath, pluginID, isRelease)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, assetHrefDefs...)
	}

	// parse bldr:esbuild comments and build import path definition list
	esbuildPkgs, err := an.FindEsbuildVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	if len(esbuildPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(esbuildPkgs), EsbuildTag)
		esbuildVarDefs, esbuildSrcFiles, err := BuildDefEsbuild(
			le,
			sourcePath,
			codeFiles,
			fset,
			esbuildPkgs,
			outAssetsPath,
			pluginID,
			isRelease,
		)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, esbuildVarDefs...)
		sourceFilesList = append(sourceFilesList, esbuildSrcFiles...)
	}

	// sort for determinism
	sort.Slice(goVariableDefs, func(i, j int) bool {
		return goVariableDefs[i].VariableName < goVariableDefs[j].VariableName
	})

	// compile Go modules
	le.Info("generating go packages")
	mc, err := NewModuleCompiler(ctx, le, workingPath, pluginID)
	if err != nil {
		return nil, nil, err
	}
	an.AddVariableDefImports(goVariableDefs)
	if err := mc.GenerateModule(an, configSetBin, goVariableDefs); err != nil {
		return nil, nil, err
	}

	outDistBinary := path.Join(outDistPath, entrypointFilename)
	if isRelease {
		le.Info("compiling release binary")
		if err := mc.CompilePlugin(outDistBinary); err != nil {
			return nil, nil, err
		}
	} else {
		le.Info("compiling dev wrapper binary")
		if err := mc.CompilePluginDevWrapper(outDistBinary, delveAddr); err != nil {
			return nil, nil, err
		}

		copyFile := func(filename string) error {
			srcPath := path.Join(mc.pluginCodegenPath, filename)
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				return nil
			}
			le.Debugf("copy %s to %s", srcPath, outDistPath)
			return fsutil.CopyFileToDir(outDistPath, srcPath, 0644)
		}

		// copy some files to dist/ which the entrypoint will need
		copyFiles := []string{
			// Don't copy go.mod, use the host program go.mod go.sum.
			// "go.mod",
			// "go.sum",
			"config-set.bin",
			"plugin.go",
		}
		for _, filename := range copyFiles {
			if err := copyFile(filename); err != nil {
				return nil, nil, err
			}
		}
	}

	sort.Strings(sourceFilesList)
	return an, sourceFilesList, nil
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
