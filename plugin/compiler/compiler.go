package plugin_compiler

import (
	"context"
	"encoding/json"
	gast "go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	cf "github.com/aperturerobotics/bldr/util/copyfile"
	util_esbuild "github.com/aperturerobotics/bldr/util/esbuild"
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
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
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
			le.Debugf("adding watcher for file: %s", filePath)
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
			configset.NewControllerConfig(1, plugin_assets_http.NewConfig("", "")),
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

	// build source files list with go files
	var sourceFilesList []string
	for _, pkgFiles := range codeFiles {
		for _, codeFile := range pkgFiles {
			pkgFile := an.GetFileToken(codeFile)
			sourceFilesList = append(sourceFilesList, pkgFile.Name())
		}
	}

	// parse esbuild comments and build import path definition list
	esbuildPkgs, err := an.ParseEsbuildComments(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	if len(esbuildPkgs) != 0 {
		le.Debugf("found %d packages with bldr:esbuild comments", len(esbuildPkgs))
	}
	var esbuildBuildOpts []*esbuild_api.BuildOptions
	var esbuildBuildVars []string
	var esbuildBuildPkgs []string
	var esbuildBuildPaths []string
	for pkgImportPath, pkgVars := range esbuildPkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, nil, errors.Errorf("failed to find corresponding ast.File for package: %s", pkgImportPath)
		}
		for pkgVar, esBuildArgs := range pkgVars {
			buildOpts := esBuildArgs.BuildOpts
			if len(buildOpts.EntryPointsAdvanced) != 0 || len(buildOpts.EntryPoints) != 1 {
				return nil, nil, errors.Errorf("%s: expected single entrypoint", pkgImportPath+"."+pkgVar)
			}

			// platform / target
			buildOpts.Platform = esbuild_api.PlatformBrowser
			buildOpts.Format = esbuild_api.FormatESModule
			if buildOpts.Target == 0 {
				buildOpts.Target = esbuild_api.ES2021
			}

			// set minify if buildMode == release
			buildOpts.MinifyWhitespace = isRelease
			buildOpts.MinifySyntax = isRelease
			buildOpts.MinifyIdentifiers = isRelease

			// TODO: add plugin to convert node_modules into plugin loads

			// other common settings
			pkgCodePath := path.Dir(an.fset.File(pkgCodeFiles[0].Pos()).Name())
			buildOpts.AbsWorkingDir = pkgCodePath
			buildOpts.LogLevel = esbuild_api.LogLevelDebug
			buildOpts.Outfile, buildOpts.Outbase = "", ""
			buildOpts.AllowOverwrite = true
			buildOpts.Bundle = true
			buildOpts.Splitting = true
			buildOpts.Metafile = true
			buildOpts.Write = true

			// output path
			buildOpts.Outdir = outAssetsPath
			esbuildBuildOpts = append(esbuildBuildOpts, buildOpts)
			esbuildBuildVars = append(esbuildBuildVars, pkgVar)
			esbuildBuildPkgs = append(esbuildBuildPkgs, pkgImportPath)
			esbuildBuildPaths = append(esbuildBuildPaths, pkgCodePath)
		}
	}
	for i, buildOpts := range esbuildBuildOpts {
		le.Debugf("compiling file(s) with esbuild: %s", buildOpts.EntryPoints)
		result := esbuild_api.Build(*buildOpts)
		if err := util_esbuild.BuildResultToErr(result); err != nil {
			return nil, nil, err
		}
		if len(result.OutputFiles) == 0 {
			return nil, nil, errors.New("esbuild: expected one output file but got none")
		}

		// Metafile contains a JSON object with information about inputs and outputs.
		type esbuildMetafile struct {
			Inputs map[string]struct {
				Bytes   int         `json:"bytes"`
				Imports interface{} `json:"imports"` // TODO
			} `json:"inputs"`
		}
		metaFile := &esbuildMetafile{}
		if err := json.Unmarshal([]byte(result.Metafile), metaFile); err != nil {
			return nil, nil, errors.Wrap(err, "parse esbuild metafile")
		}
		// Use it to get the list of source files to watch.
		// Note: the paths are relative to the package code path.
		for inFileRelPath := range metaFile.Inputs {
			inFilePath := path.Join(esbuildBuildPaths[i], inFileRelPath)
			sourceFilesList = append(sourceFilesList, inFilePath)
		}

		// metaAnalysis contains a graphical view of input files & their sizes
		metaAnalysis := esbuild_api.AnalyzeMetafile(result.Metafile, esbuild_api.AnalyzeMetafileOptions{
			Color: true,
		})
		os.Stderr.WriteString(metaAnalysis + "\n")

		// assets path is available at /p/{plugin-id}/
		relPath, err := filepath.Rel(outAssetsPath, result.OutputFiles[0].Path)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, &GoVarDef{
			PackagePath:  esbuildBuildPkgs[i],
			VariableName: esbuildBuildVars[i],
			Value: &gast.BasicLit{
				Kind: token.STRING,
				Value: strconv.Quote(strings.Join([]string{
					plugin.PluginAssetsRoute,
					pluginID,
					"/",
					relPath,
				}, "")),
			},
		})
	}

	// compile Go modules
	le.Info("generating go packages")
	mc, err := NewModuleCompiler(ctx, le, workingPath, pluginID)
	if err != nil {
		return nil, nil, err
	}
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
