package bldr_plugin_compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host_configset "github.com/aperturerobotics/bldr/plugin/host/configset"
	bldr_plugin_load "github.com/aperturerobotics/bldr/plugin/load"
	vardef "github.com/aperturerobotics/bldr/plugin/vardef"
	bldr_compress "github.com/aperturerobotics/bldr/util/compress"
	"github.com/aperturerobotics/bldr/util/enabled"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/esbuild/build"
	web_fetch_controller "github.com/aperturerobotics/bldr/web/fetch/service"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	web_pkg_fs_controller "github.com/aperturerobotics/bldr/web/pkg/fs/controller"
	web_pkg_rpc_server "github.com/aperturerobotics/bldr/web/pkg/rpc/server"
	bldr_web_plugin_handle_rpc "github.com/aperturerobotics/bldr/web/plugin/handle-rpc"
	bldr_web_plugin_handle_web_pkg "github.com/aperturerobotics/bldr/web/plugin/handle-web-pkg"
	bldr_web_plugin_handle_web_view "github.com/aperturerobotics/bldr/web/plugin/handle-web-view"
	web_runtime_wasm_build "github.com/aperturerobotics/bldr/web/runtime/wasm/build"
	web_view_handler_server "github.com/aperturerobotics/bldr/web/view/handler/server"
	bldr_web_view_observer "github.com/aperturerobotics/bldr/web/view/observer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// ControllerID is the compiler controller ID.
const ControllerID = "bldr/plugin/compiler"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "plugin compiler controller"

// Inline sourcemaps due to Chrome bug
// https://issues.chromium.org/u/1/issues/41486524#comment4 [curently open 2024/03/13]
var inlineSourcemaps = true

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
	isRelease := buildType.IsRelease()

	// output paths
	workingPath := builderConf.GetWorkingPath()
	outDistPath := filepath.Join(workingPath, "dist")
	outAssetsPath := filepath.Join(workingPath, "assets")
	outBinName := pluginID + buildPlatform.GetExecutableExt()

	// if we have an alternative entrypoint path...
	outEntrypointName := outBinName
	if entrypointExt := buildPlatform.GetEntrypointExt(); entrypointExt != "" {
		outEntrypointName = pluginID + entrypointExt
	}

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

	le := c.GetLogger().
		WithField("plugin-id", pluginID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building plugin manifest")

	// if we are in dev mode, use the dev info file for hot reload compatibility.
	var devInfoFile string
	if !isRelease {
		devInfoFile = "dev-info.bin"
	}

	// If no Go files changed, rebuild esbuild assets only (hot reload)
	prevResult := args.GetPrevBuilderResult()
	var updatedManifestMeta *manifest_builder.InputManifest
	if !prevResult.GetManifestRef().GetEmpty() && !isRelease {
		// Check out the previous result to disk.
		prevManifestRef := prevResult.GetManifestRef()
		_, err = builderConf.CheckoutManifest(
			ctx,
			le,
			busEngine.AccessWorldState,
			prevManifestRef.GetManifestRef(),
			outDistPath,
			outAssetsPath,
		)
		if err != nil {
			err = errors.Wrap(err, "failed to check out previous manifest")
		}

		// Run the fast rebuild.
		if err == nil {
			updatedManifestMeta, err = c.FastRebuildPlugin(
				ctx,
				le,
				pluginID,
				sourcePath,
				outDistPath,
				outAssetsPath,
				prevResult.GetInputManifest(),
				args.GetChangedFiles(),
				devInfoFile,
			)
		}

		if err != nil {
			le.WithError(err).Warn("fast rebuild failed: continuing with normal build")
			updatedManifestMeta = nil
		} else if updatedManifestMeta != nil {
			le.Debug("completed fast rebuild")
		}
	}

	// if fast-rebuild skipped or failed, use the full rebuild process (slower)
	if updatedManifestMeta == nil {
		// clean/create build directories
		if err := fsutil.CleanCreateDir(outDistPath); err != nil {
			return nil, err
		}
		if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
			return nil, err
		}

		// build base plugin config
		pluginBuildConf := conf.CloneVT()
		if pluginBuildConf == nil {
			pluginBuildConf = &Config{}
		}

		// apply the per-build-type configs
		pluginBuildConf.FlattenBuildTypes(buildType)

		// call any pre-build hooks
		for _, hook := range c.preBuildHooks {
			res, err := hook(ctx, builderConf, busEngine)
			if err != nil {
				return nil, err
			}

			// merge the returned config
			pluginBuildConf.Merge(res.GetConfig())
		}

		// determine project id
		projectID := builderConf.GetProjectId()
		if cproj := pluginBuildConf.GetProjectId(); cproj != "" {
			projectID = cproj
		}

		pluginMeta := bldr_plugin.NewPluginMeta(
			projectID,
			pluginID,
			buildPlatform.GetPlatformID(),
			buildType.String(),
		)

		le.Debug("compiling plugin")
		_, updatedManifestMeta, err = c.BuildPlugin(
			ctx,
			le,
			pluginMeta,
			buildType,
			buildPlatform,
			outBinName,
			workingPath,
			sourcePath,
			builderConf.GetDistSourcePath(),
			outDistPath,
			outAssetsPath,
			pluginBuildConf.GetGoPkgs(),
			pluginBuildConf.GetWebPkgs(),
			pluginBuildConf.GetWebPluginId(),
			pluginBuildConf.GetDisableRpcFetch(),
			pluginBuildConf.GetDisableFetchAssets(),
			pluginBuildConf.GetDelveAddr(),
			pluginBuildConf.GetConfigSet(),
			pluginBuildConf.GetHostConfigSet(),
			pluginBuildConf.GetEnableCgo(),
			pluginBuildConf.GetEnableTinygo(),
			pluginBuildConf.GetEnableCompression(),
			pluginBuildConf.GetEsbuildFlags(),
			devInfoFile,
		)
		if err != nil {
			return nil, err
		}
	}

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
		outEntrypointName,
		outDistPath,
		outAssetsPath,
	)
	if err != nil {
		return nil, err
	}

	le.Debugf(
		"plugin build complete with %d input files",
		len(updatedManifestMeta.Files),
	)
	result := manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		updatedManifestMeta,
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// FastRebuildPlugin compiles the plugin once skipping running the Go compiler if possible.
// Assumes we are in dev mode (not release mode).
// Assumes the previous result is already checked out to outDistPath and outAssetsPath.
// Returns nil, nil if fast rebuild is not applicable.
func (c *Controller) FastRebuildPlugin(
	ctx context.Context,
	le *logrus.Entry,
	pluginID,
	sourcePath,
	outDistPath,
	outAssetsPath string,
	prevInputManifest *manifest_builder.InputManifest,
	changedFiles []*manifest_builder.InputManifest_File,
	devInfoFile string,
) (*manifest_builder.InputManifest, error) {
	// Skip if there is no previous result.
	if len(changedFiles) == 0 || len(prevInputManifest.GetFiles()) == 0 {
		return nil, nil
	}

	// Skip if there is no valid input manifest metadata.
	prevMetaBin := prevInputManifest.Metadata
	if len(prevMetaBin) == 0 {
		return nil, nil
	}
	inputMeta := &InputManifestMeta{}
	if err := inputMeta.UnmarshalVT(prevMetaBin); err != nil {
		return nil, errors.Wrap(err, "unmarshal input metadata")
	}

	webPkgs := inputMeta.GetWebPkgs()
	baseEsbuildOpts, err := bldr_esbuild_build.ParseEsbuildFlags(inputMeta.GetEsbuildFlags())
	if err != nil {
		return nil, err
	}

	// If any non-esbuild assets changed, skip fast rebuild.
	meta := &InputFileMeta{}
	for _, changedFile := range changedFiles {
		meta.Reset()
		err := meta.UnmarshalVT(changedFile.GetMetadata())
		if err != nil {
			// parsing error
			return nil, errors.Wrap(err, "failed to parse file metadata")
		}
		if meta.GetKind() != InputFileKind_InputFileKind_ESBUILD {
			// Skip fast rebuild: non-esbuild asset
			return nil, nil
		}
	}

	// Perform fast rebuild by running the esbuild compiler only.
	le.Info("performing fast rebuild")

	// execute the build
	esbuildBundleMeta := inputMeta.GetEsbuildBundles()
	bundleIDs := maps.Keys(esbuildBundleMeta)
	slices.Sort(bundleIDs)
	var updatedWebPkgRefs []*web_pkg_esbuild.WebPkgRef
	var esbuildSrcFiles []string
	var goVariableDefs []*vardef.PluginVar
	var updatedEsbuildOutputs []*EsbuildOutputMeta
	for _, bundleID := range bundleIDs {
		bundleDef := esbuildBundleMeta[bundleID]
		esbuildVarDefs, esbuildWebPkgRefs, esbuildOutputMeta, esbuildSrcs, err := BuildEsbuildBundle(
			le,
			sourcePath,
			bundleDef,
			baseEsbuildOpts,
			webPkgs,
			outAssetsPath,
			pluginID,
			inlineSourcemaps,
			false,
		)
		if err != nil {
			return nil, err
		}

		esbuildSrcFiles = append(esbuildSrcFiles, esbuildSrcs...)
		goVariableDefs = append(goVariableDefs, esbuildVarDefs...)
		updatedEsbuildOutputs = append(updatedEsbuildOutputs, esbuildOutputMeta...)
		for _, webPkgRef := range esbuildWebPkgRefs {
			for _, impPath := range webPkgRef.Imports {
				updatedWebPkgRefs, _ = web_pkg_esbuild.WebPkgRefSlice(updatedWebPkgRefs).AppendWebPkgRef(
					webPkgRef.WebPkgId,
					webPkgRef.WebPkgRoot,
					impPath,
				)
			}
		}
	}

	// sort the web pkg refs
	web_pkg_esbuild.SortWebPkgRefs(updatedWebPkgRefs)

	// compare the web pkg refs to see if they changed.
	// if so: we must perform a full rebuild to pick up the new refs + rebuild the web pkgs.
	if !(&InputManifestMeta{WebPkgRefs: inputMeta.WebPkgRefs}).EqualVT(&InputManifestMeta{WebPkgRefs: updatedWebPkgRefs}) {
		le.Info("references to web pkgs changed: forcing a full re-build")
		return nil, nil
	}

	// cleanup esbuild src files list
	slices.Sort(esbuildSrcFiles)
	esbuildSrcFiles = slices.Compact(esbuildSrcFiles)

	// cleanup outputs list
	updatedEsbuildOutputs = SortEsbuildOutputMetas(updatedEsbuildOutputs)

	// compare the outputs list with the old outputs list.
	// delete any output file from the old outputs that was not overwritten by esbuild.
	// for example: changed files with hashes in the filename will delete the old hash.
	updatedOutputs := make(map[string]struct{}, len(updatedEsbuildOutputs))
	for _, updatedOutput := range updatedEsbuildOutputs {
		updatedOutputs[updatedOutput.GetPath()] = struct{}{}
	}
	for _, oldOutput := range inputMeta.GetEsbuildOutputs() {
		if _, ok := updatedOutputs[oldOutput.GetPath()]; !ok {
			oldOutputPath := oldOutput.GetPath()
			absPath := filepath.Join(outAssetsPath, oldOutputPath)
			relPath, err := filepath.Rel(outAssetsPath, absPath)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(relPath, "..") {
				// prevent deleting things outside the assets dir
				le.Warnf("skipping removing old output path outside assets dir: %s", relPath)
				continue
			}
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				le.Warnf("old output path not found: %s", oldOutputPath)
			} else if err := os.Remove(absPath); err != nil {
				return nil, err
			} else {
				le.Debugf("removed old output: %s", oldOutputPath)
			}
		}
	}

	// build the updated input manifest
	updatedInputManifest := prevInputManifest.CloneVT()
	updatedInputMeta := inputMeta.CloneVT()
	if updatedInputMeta.DevInfo == nil {
		updatedInputMeta.DevInfo = &vardef.PluginDevInfo{}
	}
	updatedInputMeta.EsbuildOutputs = updatedEsbuildOutputs

	// drop all esbuild files from the set (we will add them back next)
	updatedInputManifest.Files = slices.DeleteFunc(updatedInputManifest.Files, func(f *manifest_builder.InputManifest_File) bool {
		meta.Reset()
		err := meta.UnmarshalVT(f.GetMetadata())
		if err != nil {
			return false
		}
		return meta.GetKind() == InputFileKind_InputFileKind_ESBUILD
	})

	// drop all overwritten variable definitions from the set (we will add them back next)
	type varDefKey struct {
		pkgPath string
		pkgVar  string
	}
	overwrittenVarDefs := make(map[varDefKey]struct{})
	for _, goVarDef := range goVariableDefs {
		overwrittenVarDefs[varDefKey{pkgPath: goVarDef.PkgImportPath, pkgVar: goVarDef.PkgVar}] = struct{}{}
	}
	updatedInputMeta.DevInfo.PluginVars = slices.DeleteFunc(updatedInputMeta.DevInfo.PluginVars, func(goVarDef *vardef.PluginVar) bool {
		_, overwritten := overwrittenVarDefs[varDefKey{pkgPath: goVarDef.PkgImportPath, pkgVar: goVarDef.PkgVar}]
		return overwritten
	})

	// add the updated go variable defs to the list
	updatedInputMeta.DevInfo.PluginVars = append(updatedInputMeta.DevInfo.PluginVars, goVariableDefs...)
	vardef.SortPluginVars(updatedInputMeta.DevInfo.PluginVars)

	// add the updated esbuild files to the list
	if err := fsutil.ConvertPathsToRelative(sourcePath, esbuildSrcFiles); err != nil {
		return nil, err
	}
	inputFileMeta := &InputFileMeta{Kind: InputFileKind_InputFileKind_ESBUILD}
	inputFileMetaBin, err := inputFileMeta.MarshalVT()
	if err != nil {
		return nil, err
	}
	for _, srcPath := range esbuildSrcFiles {
		updatedInputManifest.Files = append(updatedInputManifest.Files, &manifest_builder.InputManifest_File{
			Path:     srcPath,
			Metadata: inputFileMetaBin,
		})
	}
	updatedInputManifest.SortFiles()

	// encode the updated meta
	updMeta, err := updatedInputMeta.MarshalVT()
	if err != nil {
		return nil, err
	}
	updatedInputManifest.Metadata = updMeta

	// encode the updated dev info file
	if devInfoFile != "" {
		updDevInfo, err := updatedInputMeta.GetDevInfo().MarshalVT()
		if err != nil {
			return nil, err
		}
		devInfoPath := filepath.Join(outDistPath, devInfoFile)
		if err := os.WriteFile(devInfoPath, updDevInfo, 0o644); err != nil {
			return nil, err
		}
		le.Debugf("wrote file: %s", devInfoFile)
	}

	le.Debug("fast rebuild complete")
	return updatedInputManifest, nil
}

// BuildPlugin compiles the plugin once, committing it to the target world.
//
// webPluginID is optional, if set, automatically adds controllers to configure the web plugin.
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
	distSourcePath,
	outDistPath,
	outAssetsPath string,
	goPkgs []string,
	webPkgs []*WebPkgRefConfig,
	webPluginID string,
	disableRpcFetch, disableFetchAssets bool,
	delveAddr string,
	configSet map[string]*configset_proto.ControllerConfig,
	hostConfigSet map[string]*configset_proto.ControllerConfig,
	enableCgoOpt enabled.Enabled,
	enableTinygoOpt enabled.Enabled,
	enableCompressionOpt enabled.Enabled,
	baseEsbuildFlags []string,
	devInfoFile string,
) (*Analysis, *manifest_builder.InputManifest, error) {
	// plugin id
	pluginID := pluginMeta.GetPluginId()
	isRelease := buildType.IsRelease()

	// clone goPkgs and webPkgs
	goPkgs = slices.Clone(goPkgs)
	webPkgs = protobuf_go_lite.CloneVTSlice(webPkgs)

	basePlatformID := buildPlatform.GetBasePlatformID()
	isNativeBuildPlatform := basePlatformID == bldr_platform.PlatformID_NATIVE
	isWebBuildPlatform := basePlatformID == bldr_platform.PlatformID_WEB

	// disable cgo on default
	enableCgo := enableCgoOpt.IsEnabled(false)
	// enable compression for release mode only on default
	enableCompression := enableCompressionOpt.IsEnabled(isRelease)
	// enable tinygo on the web platform in release mode on default
	tinygoSupported := false // TODO: TinyGo cannot yet build Bldr successfully.
	enableTinygo := isWebBuildPlatform && enableTinygoOpt.IsEnabled(isRelease && tinygoSupported)

	baseEsbuildOpts, err := bldr_esbuild_build.ParseEsbuildFlags(baseEsbuildFlags)
	if err != nil {
		return nil, nil, err
	}

	// build the config set based on configuration
	embedConfigSet := make(configset_proto.ConfigSetMap)

	// applyToConfigSet conditionally applies the config to the config set if not already set.
	applyToConfigSet := func(id string, conf config.Config) error {
		if _, ok := embedConfigSet[id]; ok {
			return nil
		}
		configBin, err := conf.MarshalVT()
		if err != nil {
			return err
		}
		embedConfigSet[id] = &configset_proto.ControllerConfig{
			Id:     conf.GetConfigID(),
			Rev:    1,
			Config: configBin,
		}
		return nil
	}

	addGoPkg := func(pkgName string) {
		if !slices.Contains(goPkgs, pkgName) {
			goPkgs = append(goPkgs, pkgName)
		}
	}

	if !disableRpcFetch {
		addGoPkg("github.com/aperturerobotics/bldr/web/fetch/service")
		if err := applyToConfigSet(
			"rpc-fetch",
			web_fetch_controller.NewConfig(),
		); err != nil {
			return nil, nil, err
		}
	}
	if !disableFetchAssets {
		addGoPkg("github.com/aperturerobotics/bldr/plugin/assets/http")
		if err := applyToConfigSet(
			"plugin-assets",
			plugin_assets_http.NewConfig(plugin.PluginAssetsHttpPrefix, ""),
		); err != nil {
			return nil, nil, err
		}
	}

	// apply the config set entries for the web plugin, if applicable.
	if webPluginID != "" {
		// - load-web: loads the web plugin on startup
		if err := applyToConfigSet("load-web", &bldr_plugin_load.Config{
			PluginId: webPluginID,
		}); err != nil {
			return nil, nil, err
		}

		// - observe-web-view: handle LookupWebView with incoming HandleWebView directives
		addGoPkg("github.com/aperturerobotics/bldr/web/view/observer")
		if err := applyToConfigSet("observe-web-view", &bldr_web_view_observer.Config{}); err != nil {
			return nil, nil, err
		}

		// - handle-web-view-rpc: handle incoming RPCs for web-view
		addGoPkg("github.com/aperturerobotics/bldr/web/plugin/handle-rpc")
		if err := applyToConfigSet("handle-web-view-rpc", &bldr_web_plugin_handle_rpc.Config{
			WebPluginId:    webPluginID,
			HandlePluginId: pluginID,
			ServerIdRe:     "web-view/.*",
		}); err != nil {
			return nil, nil, err
		}

		// - handle-web-view: handle web views via HandleWebView
		addGoPkg("github.com/aperturerobotics/bldr/web/plugin/handle-web-view")
		if err := applyToConfigSet("handle-web-view", &bldr_web_plugin_handle_web_view.Config{
			WebPluginId:    webPluginID,
			HandlePluginId: pluginID,
		}); err != nil {
			return nil, nil, err
		}

		// - handle-web-view-server: handle incoming RPCs for HandleWebView
		addGoPkg("github.com/aperturerobotics/bldr/web/view/handler/server")
		if err := applyToConfigSet("handle-web-view-server", &web_view_handler_server.Config{}); err != nil {
			return nil, nil, err
		}

		// - handle-web-pkgs: handle web pkg lookups for the webPkgIds
		if len(webPkgs) != 0 {
			// NOTE: add the actual config later after we build the web pkgs
			addGoPkg("github.com/aperturerobotics/bldr/web/plugin/handle-web-pkg")
		}
	}

	// add web pkg controllers if necessary
	if len(webPkgs) != 0 {
		addGoPkg("github.com/aperturerobotics/bldr/web/pkg/rpc/server")
		addGoPkg("github.com/aperturerobotics/bldr/web/pkg/fs/controller")
	}

	// apply host config set
	if len(hostConfigSet) != 0 {
		if err := applyToConfigSet("plugin-host-configset", &plugin_host_configset.Config{
			ConfigSet: hostConfigSet,
		}); err != nil {
			return nil, nil, err
		}
	}

	// merge configured config set entries
	configset_proto.MergeConfigSetMaps(embedConfigSet, configSet)

	// cleanup list of go packages
	slices.Sort(goPkgs)
	goPkgs = slices.Compact(goPkgs)

	// analyze go packages
	le.Info("analyzing go packages")
	buildTagsForAnalyze := gocompiler.NewBuildTags(buildType, enableCgo)
	an, err := AnalyzePackages(ctx, le, sourcePath, goPkgs, buildTagsForAnalyze)
	if err != nil {
		return nil, nil, err
	}

	// mapping between go.package.path.Variable and value
	// for the Go compiler linker flags
	var goVariableDefs []*vardef.PluginVar

	codeFiles := an.GetGoCodeFiles()
	fset := an.GetFileSet()

	// build source files list with go files
	var goSrcFiles []string
	for _, pkgFiles := range codeFiles {
		for _, codeFile := range pkgFiles {
			pkgFile := an.GetFileToken(codeFile)
			goSrcFiles = append(goSrcFiles, pkgFile.Name())
		}
	}

	// parse bldr:asset comments
	assetPkgs, err := an.FindAssetVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	var assetSrcFiles []string
	if len(assetPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(assetPkgs), AssetTag)
		assetVarDefs, assetSrcPaths, err := BuildDefAssets(le, codeFiles, fset, assetPkgs, outAssetsPath, pluginID, isRelease)
		if err != nil {
			return nil, nil, err
		}
		assetSrcFiles = assetSrcPaths
		goVariableDefs = append(goVariableDefs, assetVarDefs...)
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
	// NOTE: We specify the list of web pkgs in the parameters to BuildPlugin.
	// NOTE: However: we only actually build the web pkgs that are referenced by the code.
	// NOTE: This is because we need to tree-shake which imports are referenced.
	var webPkgRefs web_pkg_esbuild.WebPkgRefSlice

	// parse bldr:esbuild comments and build import path definition list
	esbuildPkgs, err := an.FindEsbuildVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	var esbuildSrcFiles []string
	var esbuildBundleMeta map[string]*EsbuildBundleMeta
	var esbuildOutputMeta []*EsbuildOutputMeta
	if len(esbuildPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(esbuildPkgs), EsbuildTag)

		esbuildBundleMeta, err = BuildEsbuildBundleMeta(le, sourcePath, codeFiles, fset, esbuildPkgs)
		if err != nil {
			return nil, nil, err
		}

		bundleIDs := maps.Keys(esbuildBundleMeta)
		slices.Sort(bundleIDs)
		for _, bundleID := range bundleIDs {
			bundleDef := esbuildBundleMeta[bundleID]
			esbuildVarDefs, esbuildWebPkgRefs, esbuildOutputs, esbuildSrcs, err := BuildEsbuildBundle(
				le,
				sourcePath,
				bundleDef,
				baseEsbuildOpts,
				webPkgs,
				outAssetsPath,
				pluginID,
				inlineSourcemaps,
				isRelease,
			)
			if err != nil {
				return nil, nil, err
			}

			esbuildSrcFiles = append(esbuildSrcFiles, esbuildSrcs...)
			goVariableDefs = append(goVariableDefs, esbuildVarDefs...)
			esbuildOutputMeta = append(esbuildOutputMeta, esbuildOutputs...)
			for _, webPkgRef := range esbuildWebPkgRefs {
				for _, impPath := range webPkgRef.Imports {
					webPkgRefs, _ = webPkgRefs.AppendWebPkgRef(
						webPkgRef.WebPkgId,
						webPkgRef.WebPkgRoot,
						impPath,
					)
				}
			}
		}
	}

	// cleanup esbuild src files list
	slices.Sort(esbuildSrcFiles)
	esbuildSrcFiles = slices.Compact(esbuildSrcFiles)

	// sort the web pkg refs
	web_pkg_esbuild.SortWebPkgRefs(webPkgRefs)

	// sort go variable defs
	vardef.SortPluginVars(goVariableDefs)

	// cleanup the list of outputs
	esbuildOutputMeta = SortEsbuildOutputMetas(esbuildOutputMeta)

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

	// NOTE: we add the Go pkgs to the list earlier in this function.
	if len(webPkgIDs) != 0 {
		// add the web packages rpc server to the config set.
		// resolves AccessRpcService directive
		if err := applyToConfigSet("web-pkgs-rpc", web_pkg_rpc_server.NewConfig("", webPkgIDs)); err != nil {
			return nil, nil, err
		}

		// add the web packages UnixFS-backed resolver to the config set.
		// we know the list of included web pkg ids, so provide it explicitly.
		// resolves LookupWebPkg directive
		if err := applyToConfigSet(
			"web-pkgs-fs",
			web_pkg_fs_controller.NewConfig(
				plugin.PluginAssetsFsId,
				plugin.PluginAssetsWebPkgsDir,
				true,
				webPkgIDs,
			),
		); err != nil {
			return nil, nil, err
		}

		// tell the web plugin to forward rpc requests to lookup web pkgs to our plugin.
		if webPluginID != "" {
			if err := applyToConfigSet("handle-web-pkgs", &bldr_web_plugin_handle_web_pkg.Config{
				WebPluginId:    webPluginID,
				HandlePluginId: pluginID,
				WebPkgIdList:   webPkgIDs,
			}); err != nil {
				return nil, nil, err
			}
		}
	}

	// encode config set for embedded config set binary
	var configSetBin []byte
	if len(embedConfigSet) != 0 {
		configSetObj := &configset_proto.ConfigSet{
			Configs: embedConfigSet,
		}
		configSetBin, err = configSetObj.MarshalVT()
		if err != nil {
			return nil, nil, err
		}
	}

	// compile Go modules
	le.Debug("generating go packages")
	moduleID := strings.Join([]string{pluginMeta.GetProjectId(), pluginMeta.GetPluginId()}, "-")
	mc, err := NewModuleCompiler(ctx, le, workingPath, moduleID)
	if err != nil {
		return nil, nil, err
	}
	an.AddVariableDefImports(le, goVariableDefs)

	pluginDevInfo, err := mc.GenerateModule(an, pluginMeta, configSetBin, goVariableDefs, devInfoFile)
	if err != nil {
		return nil, nil, err
	}

	copyFiles := []string{devInfoFile}
	outDistBinary := filepath.Join(outDistPath, outBinName)
	outBinNameWithoutExt := strings.TrimSuffix(outBinName, buildPlatform.GetExecutableExt())

	// only use dev wrapper if !isRelease && delveAddr != "" && platform == native
	if isRelease || delveAddr == "" || !isNativeBuildPlatform {
		le.Info("compiling plugin binary")
		if err := mc.CompilePlugin(
			ctx,
			le,
			outDistBinary,
			buildPlatform,
			buildType,
			enableCgo,
			enableTinygo,
		); err != nil {
			return nil, nil, err
		}

		// optimization pass: brotli compression
		if enableCompression && isWebBuildPlatform {
			le.Info("compressing plugin binary")

			/*
				brPath, err := bldr_compress.CompressBrotli(le, workingPath, outDistBinary)
				if err != nil {
					return nil, nil, err
				}
			*/
			brPath, err := bldr_compress.CompressGzip(le, workingPath, outDistBinary)
			if err != nil {
				return nil, nil, err
			}

			// use this new binary from now on
			if err := os.Remove(outDistBinary); err != nil {
				return nil, nil, err
			}

			outDistBinary = brPath //nolint
			outBinName = filepath.Base(brPath)
		}
	} else {
		le.Info("compiling plugin dev wrapper binary")
		if err := mc.CompilePluginDevWrapper(ctx, le, outDistBinary, delveAddr, buildPlatform, buildType, enableCgo); err != nil {
			return nil, nil, err
		}
		copyFiles = append(copyFiles, "plugin.go", "config-set.bin")
	}

	// build the WebWorker / SharedWorker js entrypoint if applicable
	if isWebBuildPlatform {
		// override entrypoint path to point to .mjs instead (for web worker)
		le.Info("compiling web plugin entrypoint")
		outScriptPath := filepath.Join(
			outDistPath,
			outBinNameWithoutExt+".mjs",
		)
		if err := web_runtime_wasm_build.BuildWebWasmPluginScript(
			ctx,
			le,
			distSourcePath,
			outScriptPath,
			outBinName,
			enableTinygo,
			isRelease,
		); err != nil {
			return nil, nil, err
		}
	}

	copyFile := func(filename string) error {
		if filename == "" {
			return nil
		}

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

		return fsutil.CopyFileToDir(outDistPath, srcPath, 0o644)
	}

	// copy some files to dist/ which the entrypoint will need
	for _, filename := range copyFiles {
		if err := copyFile(filename); err != nil {
			return nil, nil, err
		}
	}

	// sort
	SortWebPkgRefConfigs(webPkgs)
	web_pkg_esbuild.SortWebPkgRefs(webPkgRefs)

	// build manifest metadata
	inputManifestMeta := &InputManifestMeta{
		DevInfo:        pluginDevInfo,
		EsbuildBundles: esbuildBundleMeta,
		EsbuildFlags:   baseEsbuildFlags,
		EsbuildOutputs: esbuildOutputMeta,
		WebPkgRefs:     webPkgRefs,
		WebPkgs:        webPkgs,
	}
	inputManifestMetaBin, err := inputManifestMeta.MarshalVT()
	if err != nil {
		return nil, nil, err
	}

	inputManifest := &manifest_builder.InputManifest{Metadata: inputManifestMetaBin}
	inputFileKinds := map[InputFileKind][]string{
		InputFileKind_InputFileKind_GO:      goSrcFiles,
		InputFileKind_InputFileKind_ASSET:   assetSrcFiles,
		InputFileKind_InputFileKind_ESBUILD: esbuildSrcFiles,
		InputFileKind_InputFileKind_WEB_PKG: webPkgSrcFiles,
	}
	for kind, srcPaths := range inputFileKinds {
		meta := &InputFileMeta{Kind: kind}
		metaBin, err := meta.MarshalVT()
		if err != nil {
			return nil, nil, err
		}

		err = fsutil.ConvertPathsToRelative(sourcePath, srcPaths)
		if err != nil {
			return nil, nil, err
		}

		for _, srcPath := range srcPaths {
			inputManifest.Files = append(inputManifest.Files, &manifest_builder.InputManifest_File{
				Path:     srcPath,
				Metadata: metaBin,
			})
		}
	}
	inputManifest.SortFiles()

	return an, inputManifest, nil
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
