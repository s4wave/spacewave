package bldr_plugin_compiler

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host_configset "github.com/aperturerobotics/bldr/plugin/host/configset"
	"github.com/aperturerobotics/bldr/util/fsutil"
	web_fetch_controller "github.com/aperturerobotics/bldr/web/fetch/service"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	web_pkg_fs_controller "github.com/aperturerobotics/bldr/web/pkg/fs/controller"
	web_pkg_rpc_server "github.com/aperturerobotics/bldr/web/pkg/rpc/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/blang/semver"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
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
	preBuildHooks []PreBuildHook
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewController constructs a new plugin compiler controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return &Controller{
		BusController: bus.NewBusController(
			le,
			b,
			conf,
			ControllerID,
			Version,
			controllerDescrip,
		),
	}, nil
}

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

// PreBuildHook is a callback called before building the plugin.
// Returns an optional PreBuildResult.
type PreBuildHook func(
	ctx context.Context,
	builderConf *manifest_builder.BuilderConfig,
	worldEng world.Engine,
) (*PreBuildHookResult, error)

// AddPreBuildHook adds a callback that is called just after constructing the plugin working dir.
// Called before calling the Go compiler or bundling the assets or dist fs.
// NOTE: may be removed in future
func (c *Controller) AddPreBuildHook(hook PreBuildHook) {
	if hook != nil {
		c.preBuildHooks = append(c.preBuildHooks, hook)
	}
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// BuildManifest compiles the manifest once with the given builder args.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *manifest_builder.BuildManifestArgs,
) (*manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	platformID := meta.GetPlatformId()
	pluginID := strings.TrimSpace(meta.GetManifestId())

	sourcePath := builderConf.GetSourcePath()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())

	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building plugin manifest")

	// clean / create dist dir
	workingPath := builderConf.GetWorkingPath()
	outDistPath := filepath.Join(workingPath, "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}

	// clean / create assets dir
	outAssetsPath := filepath.Join(workingPath, "assets")
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		return nil, err
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

	// build base plugin config
	pluginBuildConf := conf.CloneVT()
	if pluginBuildConf == nil {
		pluginBuildConf = &Config{}
	}

	// call any pre-build hooks
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, busEngine)
		if err != nil {
			return nil, err
		}

		// merge the returned config
		pluginBuildConf.Merge(res.GetConfig())
	}

	// clone the config set maps
	configSet := make(map[string]*configset_proto.ControllerConfig, len(pluginBuildConf.GetConfigSet()))
	for k, v := range pluginBuildConf.GetConfigSet() {
		configSet[k] = v.CloneVT()
	}

	hostConfigSet := make(map[string]*configset_proto.ControllerConfig, len(pluginBuildConf.GetHostConfigSet()))
	for k, v := range pluginBuildConf.GetHostConfigSet() {
		hostConfigSet[k] = v.CloneVT()
	}

	// apply host config set
	if len(hostConfigSet) != 0 {
		hostConfigSetConf, err := jsonpb.Marshal(&plugin_host_configset.Config{
			ConfigSet: hostConfigSet,
		})
		if err != nil {
			if err != context.Canceled {
				return nil, err
			}
			return nil, errors.Wrap(err, "marshal plugin host configset")
		}

		configSet["plugin-host-configset"] = &configset_proto.ControllerConfig{
			Id:     plugin_host_configset.ConfigID,
			Rev:    1,
			Config: hostConfigSetConf,
		}
	}

	// determine project id
	projectID := builderConf.GetProjectId()
	if cproj := pluginBuildConf.GetProjectId(); cproj != "" {
		projectID = cproj
	}

	// Cleanup list of go packages
	goPkgs := slices.Clone(pluginBuildConf.GetGoPkgs())
	sort.Strings(goPkgs)
	goPkgs = slices.Compact(goPkgs)

	// Cleanup list of web packages
	webPkgs := slices.Clone(pluginBuildConf.GetWebPkgs())
	sort.Strings(webPkgs)
	webPkgs = slices.Compact(webPkgs)

	// Enable cgo only if flag is set (for reproducible builds)
	enableCgo := pluginBuildConf.GetEnableCgo()

	baseEsbuildOpts, err := pluginBuildConf.ParseEsbuildFlags()
	if err != nil {
		return nil, err
	}

	// TODO: if no Go files changed, rebuild esbuild assets only (hot reload)
	/*
		prevResult := args.GetPrevBuilderResult()
		if prevResult != nil {
			prevManifest := prevResult.GetManifest()
			changedFiles := args.GetChangedFiles()
		}
	*/

	le.Debug("compiling plugin")
	outBinName := pluginID + buildPlatform.GetExecutableExt()
	pluginMeta := bldr_plugin.NewPluginMeta(projectID, pluginID, buildPlatform.GetPlatformID())
	_, consumedSrcFiles, err := c.BuildPlugin(
		ctx,
		le,
		pluginMeta,
		buildType,
		buildPlatform,
		outBinName,
		workingPath,
		sourcePath,
		outDistPath,
		outAssetsPath,
		goPkgs,
		webPkgs,
		pluginBuildConf.GetDisableRpcFetch(),
		pluginBuildConf.GetDisableFetchAssets(),
		pluginBuildConf.GetDelveAddr(),
		configSet,
		enableCgo,
		baseEsbuildOpts,
	)
	if err != nil {
		return nil, err
	}

	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	le.Debug("bundling plugin files")
	// bundle dist and assets fs
	distFs, assetsFs := os.DirFS(outDistPath), os.DirFS(outAssetsPath)
	committedManifest, committedManifestRef, err := builderConf.CommitManifest(
		ctx,
		le,
		tx,
		meta,
		outBinName,
		distFs,
		assetsFs,
	)
	if err != nil {
		return nil, err
	}

	// convert paths to relative
	err = fsutil.ConvertPathsToRelative(sourcePath, consumedSrcFiles)
	if err != nil {
		return nil, errors.Wrap(err, "source paths")
	}

	le.Debugf(
		"plugin build complete: %d go packages with %d files",
		len(goPkgs),
		len(consumedSrcFiles),
	)
	result := manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		manifest_builder.NewInputManifest(consumedSrcFiles),
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// BuildPlugin compiles the plugin once, committing it to the target world.
//
// Returns a list of source files from the list of given goPkgs.
// Source files list includes all files consumed by esbuild.
func (c *Controller) BuildPlugin(
	ctx context.Context,
	le *logrus.Entry,
	pluginMeta *bldr_plugin.PluginMeta,
	buildType bldr_manifest.BuildType,
	buildPlatform bldr_platform.Platform,
	outBinName,
	workingPath,
	sourcePath,
	outDistPath,
	outAssetsPath string,
	goPkgs, webPkgs []string,
	disableRpcFetch, disableFetchAssets bool,
	delveAddr string,
	configSet map[string]*configset_proto.ControllerConfig,
	enableCgo bool,
	baseEsbuildOpts *esbuild_api.BuildOptions,
) (*Analysis, []string, error) {
	// plugin id
	pluginID := pluginMeta.GetPluginId()

	// build the config set based on configuration
	embedConfigSet := make(configset_proto.ConfigSetMap)
	var err error
	if !disableRpcFetch {
		embedConfigSet["rpc-fetch"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, web_fetch_controller.NewConfig()),
			false,
		)
		if err != nil {
			return nil, nil, err
		}
	}
	if !disableFetchAssets {
		embedConfigSet["plugin-assets"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, plugin_assets_http.NewConfig(plugin.PluginAssetsHttpPrefix, "")),
			false,
		)
		if err != nil {
			return nil, nil, err
		}
	}

	// merge configured config set entries
	configset_proto.MergeConfigSetMaps(embedConfigSet, configSet)

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

	// track web pkg refs
	var webPkgRefs []*web_pkg_esbuild.WebPkgRef

	// parse bldr:esbuild comments and build import path definition list
	esbuildPkgs, err := an.FindEsbuildVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	if len(esbuildPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(esbuildPkgs), EsbuildTag)

		esbuildVarDefs, esbuildWebPkgRefs, esbuildSrcFiles, err := BuildDefEsbuild(
			le,
			sourcePath,
			codeFiles,
			fset,
			baseEsbuildOpts,
			esbuildPkgs,
			webPkgs,
			outAssetsPath,
			pluginID,
			isRelease,
		)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, esbuildVarDefs...)
		sourceFilesList = append(sourceFilesList, esbuildSrcFiles...)
		for _, webPkgRef := range esbuildWebPkgRefs {
			for _, impPath := range webPkgRef.Imports {
				webPkgRefs = web_pkg_esbuild.AddWebPkgRef(webPkgRefs, webPkgRef.WebPkgID, webPkgRef.WebPkgRoot, impPath)
			}
		}
	}

	// sort for determinism
	sort.Slice(goVariableDefs, func(i, j int) bool {
		return goVariableDefs[i].VariableName < goVariableDefs[j].VariableName
	})

	// run esbuild on the web pkgs (if any)
	outWebPkgsPath := filepath.Join(outAssetsPath, plugin.PluginAssetsWebPkgsDir)
	webPkgIDs, webPkgSrcFiles, err := web_pkg_esbuild.BuildWebPkgsEsbuild(
		ctx,
		le,
		sourcePath,
		webPkgRefs,
		outWebPkgsPath,
		plugin.PluginWebPkgHttpPrefix,
		isRelease,
	)
	if err != nil {
		return nil, nil, err
	}
	sourceFilesList = append(sourceFilesList, webPkgSrcFiles...)

	if len(webPkgIDs) != 0 {
		// add the web packages rpc server to the config set.
		// resolves AccessRpcService directive
		embedConfigSet["web-pkgs-rpc"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, web_pkg_rpc_server.NewConfig("", webPkgIDs)),
			false,
		)
		if err != nil {
			return nil, nil, err
		}

		// add the web packages UnixFS-backed resolver to the config set.
		// we know the list of included web pkg ids, so provide it explicitly.
		// resolves LookupWebPkg directive
		embedConfigSet["web-pkgs-fs"], err = configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, web_pkg_fs_controller.NewConfig(
				plugin.PluginAssetsFsId,
				plugin.PluginAssetsWebPkgsDir,
				true,
				webPkgIDs,
			)),
			false,
		)
		if err != nil {
			return nil, nil, err
		}

		// tell the web plugin to forward rpc requests to lookup web pkgs to our plugin.
		/*
				embedConfigSet["web-pkgs-fwd"], err = configset_proto.NewControllerConfig(
					configset.NewControllerConfig(1, &bldr_web_plugin_handle_rpc.Config{
			        	...,
					}),
					false,
				)
				if err != nil {
					return nil, nil, err
				}
		*/
	}

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

	// compile Go modules
	le.Info("generating go packages")
	moduleID := strings.Join([]string{pluginMeta.GetProjectId(), pluginMeta.GetPluginId()}, "-")
	mc, err := NewModuleCompiler(ctx, le, workingPath, moduleID)
	if err != nil {
		return nil, nil, err
	}
	an.AddVariableDefImports(le, goVariableDefs)
	if err := mc.GenerateModule(an, pluginMeta, configSetBin, goVariableDefs); err != nil {
		return nil, nil, err
	}

	outDistBinary := filepath.Join(outDistPath, outBinName)
	if isRelease {
		le.Info("compiling release binary")
		if err := mc.CompilePlugin(ctx, le, outDistBinary, buildPlatform, enableCgo); err != nil {
			return nil, nil, err
		}
	} else {
		le.Info("compiling dev wrapper binary")
		if err := mc.CompilePluginDevWrapper(ctx, le, outDistBinary, delveAddr, enableCgo); err != nil {
			return nil, nil, err
		}

		copyFile := func(filename string) error {
			srcPath := filepath.Join(mc.pluginCodegenPath, filename)
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				return nil
			}

			// log relative to cwd
			relSrcPath, relOutDistPath := srcPath, outDistPath
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				if rs, err := filepath.Rel(cwd, relSrcPath); err == nil {
					relSrcPath = rs
				}
				if rs, err := filepath.Rel(cwd, relOutDistPath); err == nil {
					relOutDistPath = rs
				}
			}
			le.Debugf("copy %s to %s", relSrcPath, relOutDistPath)

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
