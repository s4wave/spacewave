package plugin_compiler

import (
	"context"
	"os"
	"path"
	"sort"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	cf "github.com/aperturerobotics/bldr/util/copyfile"
	web_fetch_controller "github.com/aperturerobotics/bldr/web/fetch/service"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/blang/semver"
	"github.com/fsnotify/fsnotify"
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
	buildType := plugin.ToBuildType(builderConf.GetBuildType())
	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType)

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
	outAssetsPath := path.Join(builderConf.GetWorkingPath(), "web")
	if err := cleanCreateDir(outAssetsPath); err != nil {
		return err
	}

	le.Info("checking module file")
	err := MaybeRunGoModTidy(ctx, le, sourcePath)
	if err != nil {
		return err
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), conf.GetPluginBuilderConfig().GetEngineId())
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
			conf.GetConfigSet(),
		)
		if err != nil {
			return err
		}

		le.Debug("bundling plugin files")
		ts := timestamp.Now()
		opPeerID, err := c.GetConfig().GetPluginBuilderConfig().ParsePeerID()
		if err != nil {
			return err
		}
		_, err = c.CommitPluginManifest(
			ctx,
			le,
			busEngine,
			pluginID,
			buildType,
			entrypointFilename,
			outDistPath,
			outAssetsPath,
			opPeerID,
			&ts,
		)
		if err != nil {
			return err
		}

		le.Info("plugin build complete")
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
			le.Debugf("removing watcher for file: %s", filePath)
			if err := watcher.Remove(filePath); err != nil {
				return err
			}
		}
		for filePath := range nextWatchedFiles {
			watchedFiles[filePath] = struct{}{}
			if err := watcher.Add(filePath); err != nil {
				return err
			}
		}

		le.Debugf(
			"hot: watching %d packages with %d files",
			len(goPkgs),
			len(watchedFiles),
		)

		// wait for a file change
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
	buildType plugin.BuildType,
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
			return cf.CopyFileToDir(outDistPath, srcPath, 0644)
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

// CommitPluginManifest commits the plugin manifest with output paths.
func (c *Controller) CommitPluginManifest(
	ctx context.Context,
	le *logrus.Entry,
	engine world.Engine,
	pluginID string,
	buildType plugin.BuildType,
	entrypointFilename string,
	outDistPath, outAssetsPath string,
	opPeerID peer.ID,
	ts *timestamp.Timestamp,
) (*plugin.PluginManifest, error) {
	// bundle dist directory
	distFs := os.DirFS(outDistPath)
	webAssetsFs := os.DirFS(outAssetsPath)
	var manifest *plugin.PluginManifest
	manifestRef, err := world.AccessObject(ctx, engine.AccessWorldState, nil, func(bcs *block.Cursor) (err error) {
		manifest, err = plugin.CreatePluginManifest(
			ctx,
			bcs,
			pluginID,
			entrypointFilename,
			distFs,
			webAssetsFs,
			buildType,
			ts,
		)
		return err
	})
	if err != nil {
		return nil, err
	}

	le.Infof("committing plugin manifest to world: %s", manifestRef.MarshalString())
	tx, err := engine.NewTransaction(true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	_, _, err = tx.ApplyWorldOp(
		plugin_host.NewUpdatePluginManifestOp(
			c.GetConfig().GetPluginBuilderConfig().GetPluginHostKey(),
			pluginID,
			manifestRef,
		),
		opPeerID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return manifest, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
