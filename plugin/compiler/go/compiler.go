//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_host_configset "github.com/aperturerobotics/bldr/plugin/host/configset"
	bldr_plugin_load "github.com/aperturerobotics/bldr/plugin/load"
	vardef "github.com/aperturerobotics/bldr/plugin/vardef"
	bldr_project "github.com/aperturerobotics/bldr/project"
	bldr_compress "github.com/aperturerobotics/bldr/util/compress"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_bundler_esbuild "github.com/aperturerobotics/bldr/web/bundler/esbuild"
	bldr_web_bundler_esbuild_compiler "github.com/aperturerobotics/bldr/web/bundler/esbuild/compiler"
	bldr_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/aperturerobotics/bldr/web/bundler/vite/compiler"
	web_fetch_controller "github.com/aperturerobotics/bldr/web/fetch/service"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
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
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/hydra/world"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/util/enabled"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "go plugin compiler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
	preBuildHooks []PreBuildHook
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewControllerWithBusController constructs a new plugin compiler controller with an existing BusController.
func NewControllerWithBusController(base *bus.BusController[*Config]) (*Controller, error) {
	return &Controller{
		BusController: base,
	}, nil
}

// NewController constructs a new plugin compiler controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	base := bus.NewBusController(
		le,
		b,
		conf,
		ControllerID,
		Version,
		controllerDescrip,
	)

	return NewControllerWithBusController(base)
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
		NewControllerWithBusController,
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

// BuildManifest compiles the manifest with the given builder args.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *manifest_builder.BuildManifestArgs,
	buildHost bldr_manifest_builder.BuildManifestHost,
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
	distSourcePath := builderConf.GetDistSourcePath()

	// if we have an alternative entrypoint path...
	outEntrypointName := outBinName
	if entrypointExt := buildPlatform.GetEntrypointExt(); entrypointExt != "" {
		outEntrypointName = pluginID + entrypointExt
	}

	// build output world engine
	buildWorld := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

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
			buildWorld.AccessWorldState,
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
				distSourcePath,
				workingPath,
				outDistPath,
				outAssetsPath,
				conf.GetEsbuildFlags(),
				prevResult.GetInputManifest(),
				args.GetChangedFiles(),
				devInfoFile,
				builderConf,
				buildHost,
				buildWorld,
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
			res, err := hook(ctx, builderConf, buildWorld)
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
			buildWorld,
			buildHost,
			buildType,
			buildPlatform,
			outBinName,
			workingPath,
			sourcePath,
			distSourcePath,
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

	tx, err := buildWorld.NewTransaction(ctx, true)
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

// BuildPlugin compiles the plugin once, committing it to the target world.
//
// webPluginID is optional, if set, automatically adds controllers to configure the web plugin.
// Returns a list of source files from the list of given goPkgs.
// Source files list includes all files consumed by esbuild.
// This is the main function that orchestrates the entire plugin build process.
func (c *Controller) BuildPlugin(
	ctx context.Context,
	le *logrus.Entry,
	pluginMeta *bldr_plugin.PluginMeta,
	buildWorld world.Engine,
	buildHost bldr_manifest_builder.BuildManifestHost,
	buildType bldr_manifest.BuildType,
	buildPlatform bldr_platform.Platform,
	outBinName,
	workingPath,
	sourcePath,
	distSourcePath,
	outDistPath,
	outAssetsPath string,
	goPkgs []string,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
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
	conf := c.GetConfig()

	// clone goPkgs and webPkgs
	goPkgs = slices.Clone(goPkgs)
	webPkgs = protobuf_go_lite.CloneVTSlice(webPkgs)

	basePlatformID := buildPlatform.GetBasePlatformID()
	isNativeBuildPlatform := basePlatformID == bldr_platform.PlatformID_NATIVE
	isWebBuildPlatform := basePlatformID == bldr_platform.PlatformID_WEB

	// disable cgo on default (false means default value is false)
	enableCgo := enableCgoOpt.IsEnabled(false)
	// enable compression for release mode only on default (isRelease means default value depends on release mode)
	enableCompression := enableCompressionOpt.IsEnabled(isRelease)
	// enable tinygo on the web platform in release mode on default
	tinygoSupported := false // TODO: TinyGo cannot yet build Bldr successfully.
	// Only enable TinyGo if: 1) we're building for web, 2) user explicitly enabled it or it's release mode, 3) TinyGo is supported
	enableTinygo := isWebBuildPlatform && enableTinygoOpt.IsEnabled(isRelease && tinygoSupported)

	// build the config set based on configuration
	embedConfigSet := make(configset_proto.ConfigSetMap)

	// applyToConfigSet applies the config to the target config set if it does not already exist
	applyToConfigSet := func(id string, conf config.Config) error {
		if _, ok := embedConfigSet[id]; ok {
			return nil // Skip if this config ID already exists in the map
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

		// - handle-web-pkgs: handle web pkg lookups for the webPkgIds if there are any webPkgs defined
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

	// ensure all go packages were found.
	for srcPkg, dstPkg := range an.GetPackagePathMappings() {
		if _, ok := an.GetLoadedPackages()[dstPkg]; !ok {
			return nil, nil, errors.Errorf("go package not found: make sure it is imported in at least one Go file: %v", srcPkg)
		}
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
	var webPkgRefs web_pkg.WebPkgRefSlice

	// parse bldr:esbuild comments and build import path definition list
	esbuildPkgs, err := an.FindEsbuildVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	var esbuildBundleVarMeta []*EsbuildBundleVarMeta
	var esbuildOutputMeta []*bldr_web_bundler_esbuild.EsbuildOutputMeta
	if len(esbuildPkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(esbuildPkgs), EsbuildTag)

		// esbuildBundleVarMeta is sorted
		esbuildBundleVarMeta, err = BuildEsbuildBundleVarMeta(le, sourcePath, codeFiles, fset, esbuildPkgs)
		if err != nil {
			return nil, nil, err
		}

		// Build and checkout the esbuild sub-manifest
		webPkgRefs, esbuildOutputMeta, err = c.buildAndCheckoutEsbuildSubManifest(
			ctx,
			le,
			buildHost,
			buildWorld,
			pluginID,
			sourcePath,
			outAssetsPath,
			esbuildBundleVarMeta,
			webPkgs,
			baseEsbuildFlags,
		)
		if err != nil {
			return nil, nil, err
		}

		// build the go variable bindings to the js files
		esbuildGoVarDefs, err := buildEsbuildGoVariableDefs(pluginID, esbuildBundleVarMeta, esbuildOutputMeta)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, esbuildGoVarDefs...)
	}

	// parse bldr:vite comments and build import path definition list
	vitePkgs, err := an.FindViteVariables(codeFiles)
	if err != nil {
		return nil, nil, err
	}
	var viteBundleVarMeta []*ViteBundleVarMeta
	var viteOutputMeta []*bldr_vite.ViteOutputMeta
	var viteWebPkgRefs web_pkg.WebPkgRefSlice
	if len(vitePkgs) != 0 {
		le.Debugf("found %d packages with %s comments", len(vitePkgs), ViteTag)

		// viteBundleVarMeta is sorted
		viteBundleVarMeta, err = BuildViteBundleVarMeta(le, sourcePath, codeFiles, fset, vitePkgs)
		if err != nil {
			return nil, nil, err
		}

		// Get base vite config paths from the config
		viteConfigPaths := conf.GetViteConfigPaths()
		disableProjectConfig := conf.GetViteDisableProjectConfig()

		// Build and checkout the vite sub-manifest
		viteWebPkgRefs, viteOutputMeta, err = c.buildAndCheckoutViteSubManifest(
			ctx,
			le,
			buildHost,
			buildWorld,
			outAssetsPath,
			viteBundleVarMeta,
			webPkgs,
			viteConfigPaths,
			disableProjectConfig,
		)
		if err != nil {
			return nil, nil, err
		}

		// build the go variable bindings to the output files
		viteGoVarDefs, err := buildViteGoVariableDefs(pluginID, viteBundleVarMeta, viteOutputMeta)
		if err != nil {
			return nil, nil, err
		}
		goVariableDefs = append(goVariableDefs, viteGoVarDefs...)

		// Merge web pkg refs from vite with any from esbuild
		webPkgRefs = append(webPkgRefs, viteWebPkgRefs...)
	}

	// sort the web pkg refs
	web_pkg.SortWebPkgRefs(webPkgRefs)

	// sort go variable defs
	vardef.SortPluginVars(goVariableDefs)

	// NOTE: we add the Go pkgs to the list earlier in this function.
	webPkgIDs := webPkgRefs.ToWebPkgIDList()
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

	// Write dev info file if applicable.
	if err := writeDevInfoFile(le, outDistPath, devInfoFile, pluginDevInfo); err != nil {
		return nil, nil, errors.Wrap(err, "write dev info file")
	}

	// Files to copy from the generated module directory to the output dist directory.
	var copyFiles []string
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

		// optimization pass: compression
		if enableCompression && isWebBuildPlatform {
			le.Info("compressing plugin binary")

			/*
				brPath, err := bldr_compress.CompressBrotli(le, workingPath, outDistBinary)
				if err != nil {
					return nil, nil, err
				}
			*/

			brPath, err := bldr_compress.CompressGzip(ctx, le, workingPath, outDistBinary)
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
	web_pkg.SortWebPkgRefs(webPkgRefs)

	// sort and compact
	webPkgs = bldr_web_bundler.CompactWebPkgRefConfigs(slices.Clone(webPkgs))

	// build manifest metadata
	inputManifestMeta := &InputManifestMeta{
		DevInfo: pluginDevInfo,

		WebPkgRefs: webPkgRefs,
		WebPkgs:    webPkgs,

		EsbuildBundles: esbuildBundleVarMeta,
		EsbuildFlags:   baseEsbuildFlags,
		EsbuildOutputs: esbuildOutputMeta,

		ViteBundles:              viteBundleVarMeta,
		ViteConfigPaths:          conf.GetViteConfigPaths(),
		ViteOutputs:              viteOutputMeta,
		ViteDisableProjectConfig: conf.GetViteDisableProjectConfig(),
	}
	inputManifestMetaBin, err := inputManifestMeta.MarshalVT()
	if err != nil {
		return nil, nil, err
	}

	inputManifest := &manifest_builder.InputManifest{Metadata: inputManifestMetaBin}
	inputFileKinds := map[InputFileKind][]string{
		InputFileKind_InputFileKind_GO:    goSrcFiles,
		InputFileKind_InputFileKind_ASSET: assetSrcFiles,
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

// FastRebuildPlugin compiles the plugin once skipping running the Go compiler if possible.
// Assumes we are in dev mode (not release mode).
// Assumes the previous result is already checked out to outDistPath and outAssetsPath.
// Returns nil, nil if fast rebuild is not applicable.
func (c *Controller) FastRebuildPlugin(
	ctx context.Context,
	le *logrus.Entry,
	pluginID,
	sourcePath,
	distSourcePath,
	workingPath,
	outDistPath,
	outAssetsPath string,
	baseEsbuildFlags []string,
	prevInputManifest *manifest_builder.InputManifest,
	changedFiles []*manifest_builder.InputManifest_File,
	devInfoFile string,
	builderConf *manifest_builder.BuilderConfig,
	buildHost bldr_manifest_builder.BuildManifestHost,
	buildWorld world.Engine,
) (*manifest_builder.InputManifest, error) {
	// Skip if there is no previous result.
	if len(prevInputManifest.GetFiles()) == 0 {
		return nil, nil
	}

	// If any Go or Asset files changed, skip fast rebuild.
	// The manifest builder will be restarted when the sub-manifest changes.
	// Those changed files won't appear in the changedFiles set.
	// Therefore if changedFiles has anything in it, we changed an Asset or a Go file.
	if len(changedFiles) != 0 {
		// Skip fast rebuild: non-esbuild asset changed.
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

	// If nothing was rebuilt, return
	prevEsbuildBundles := inputMeta.GetEsbuildBundles()
	prevViteBundles := inputMeta.GetViteBundles()
	if len(prevEsbuildBundles) == 0 && len(prevViteBundles) == 0 {
		// Nothing to rebuild
		return nil, nil
	}

	// Perform fast rebuild by running the bundlers only.
	le.Info("performing fast rebuild")

	prevWebPkgs := inputMeta.GetWebPkgs()
	var updatedWebPkgRefs web_pkg.WebPkgRefSlice
	var esbuildWebPkgRefs web_pkg.WebPkgRefSlice
	var viteWebPkgRefs web_pkg.WebPkgRefSlice
	var updatedEsbuildOutputs []*bldr_web_bundler_esbuild.EsbuildOutputMeta
	var updatedViteOutputs []*bldr_vite.ViteOutputMeta

	// Check for esbuild bundles to rebuild
	if len(prevEsbuildBundles) > 0 {
		// Build esbuild config based on previous metadata
		publicPath := BuildAssetHref(pluginID, EsbuildAssetSubdir)
		esbuildBundlerConf, err := BuildEsbuildBundlerConfig(prevEsbuildBundles, prevWebPkgs, baseEsbuildFlags, sourcePath, publicPath)
		if err == nil {
			err = esbuildBundlerConf.Validate()
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to build esbuild bundler config for fast rebuild")
		}

		// Build and checkout the esbuild sub-manifest
		// Capture the esbuild output metadata for later use.
		esbuildWebPkgRefs, updatedEsbuildOutputs, err = c.buildAndCheckoutEsbuildSubManifest(
			ctx,
			le,
			buildHost,
			buildWorld,
			pluginID,
			sourcePath,
			outAssetsPath,
			prevEsbuildBundles, // Use previous bundles for config
			prevWebPkgs,        // Use previous web pkgs for config
			baseEsbuildFlags,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build and checkout esbuild sub-manifest during fast rebuild")
		}

		// Add to collected web pkg refs
		updatedWebPkgRefs = append(updatedWebPkgRefs, esbuildWebPkgRefs...)
	}

	// Check for vite bundles to rebuild
	prevViteConfigPaths := inputMeta.GetViteConfigPaths()
	prevViteDisableProjectConfig := inputMeta.GetViteDisableProjectConfig()

	if len(prevViteBundles) > 0 {
		// Build vite config based on previous metadata
		viteBundlerConf, err := BuildViteBundlerConfig(prevViteBundles, prevWebPkgs, prevViteConfigPaths, prevViteDisableProjectConfig)
		if err == nil {
			err = viteBundlerConf.Validate()
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to build vite bundler config for fast rebuild")
		}

		// Build and checkout the vite sub-manifest
		// Capture the vite output metadata for later use.
		viteWebPkgRefs, updatedViteOutputs, err = c.buildAndCheckoutViteSubManifest(
			ctx,
			le,
			buildHost,
			buildWorld,
			outAssetsPath,
			prevViteBundles,
			prevWebPkgs,
			prevViteConfigPaths,
			prevViteDisableProjectConfig,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build and checkout vite sub-manifest during fast rebuild")
		}

		// Add to collected web pkg refs
		updatedWebPkgRefs = append(updatedWebPkgRefs, viteWebPkgRefs...)
	}

	// Sort web pkg refs for comparison
	web_pkg.SortWebPkgRefs(updatedWebPkgRefs)

	// Compare the web pkg refs to see if they changed.
	// If so: we must perform a full rebuild to pick up the new refs + rebuild the web pkgs.
	if !(&InputManifestMeta{WebPkgRefs: inputMeta.WebPkgRefs}).EqualVT(&InputManifestMeta{WebPkgRefs: updatedWebPkgRefs}) {
		le.Info("references to web pkgs changed: forcing a full re-build")
		return nil, nil
	}

	// Build the go variable bindings based on the *new* outputs and *old* bundle definitions
	var nextGoVariableDefs []*vardef.PluginVar

	// Process esbuild variable definitions if we have any
	if len(prevEsbuildBundles) > 0 {
		esbuildVarDefs, err := buildEsbuildGoVariableDefs(pluginID, prevEsbuildBundles, updatedEsbuildOutputs)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build esbuild go variable definitions during fast rebuild")
		}
		nextGoVariableDefs = append(nextGoVariableDefs, esbuildVarDefs...)
	}

	// Process vite variable definitions if we have any
	if len(prevViteBundles) > 0 {
		viteVarDefs, err := buildViteGoVariableDefs(pluginID, prevViteBundles, updatedViteOutputs)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build vite go variable definitions during fast rebuild")
		}
		nextGoVariableDefs = append(nextGoVariableDefs, viteVarDefs...)
	}

	vardef.SortPluginVars(nextGoVariableDefs)

	// Build the updated input manifest
	updatedInputManifest := prevInputManifest.CloneVT()
	updatedInputMeta := inputMeta.CloneVT()
	if updatedInputMeta.DevInfo == nil {
		// Ensure DevInfo exists, though it should if we got this far from a previous build
		updatedInputMeta.DevInfo = &vardef.PluginDevInfo{}
	}

	// Update outputs in the metadata
	if len(updatedEsbuildOutputs) > 0 {
		updatedInputMeta.EsbuildOutputs = updatedEsbuildOutputs
	}
	if len(updatedViteOutputs) > 0 {
		updatedInputMeta.ViteOutputs = updatedViteOutputs
	}
	// WebPkgRefs are confirmed to be the same, no need to update updatedInputMeta.WebPkgRefs

	// Drop all overwritten variable definitions from the DevInfo set (we will add them back next)
	type varDefKey struct {
		pkgPath string
		pkgVar  string
	}
	overwrittenVarDefs := make(map[varDefKey]struct{})
	for _, goVarDef := range nextGoVariableDefs {
		overwrittenVarDefs[varDefKey{pkgPath: goVarDef.PkgImportPath, pkgVar: goVarDef.PkgVar}] = struct{}{}
	}
	updatedInputMeta.DevInfo.PluginVars = slices.DeleteFunc(updatedInputMeta.DevInfo.PluginVars, func(goVarDef *vardef.PluginVar) bool {
		_, overwritten := overwrittenVarDefs[varDefKey{pkgPath: goVarDef.PkgImportPath, pkgVar: goVarDef.PkgVar}]
		return overwritten
	})

	// Add the updated go variable defs to the list
	updatedInputMeta.DevInfo.PluginVars = append(updatedInputMeta.DevInfo.PluginVars, nextGoVariableDefs...)
	vardef.SortPluginVars(updatedInputMeta.DevInfo.PluginVars)

	// Sort updated input manifest files (Go and Asset files remain)
	updatedInputManifest.SortFiles()

	// Encode the updated meta
	updMetaBin, err := updatedInputMeta.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal updated input meta for fast rebuild")
	}
	updatedInputManifest.Metadata = updMetaBin

	// Write the updated dev info file if applicable.
	if err := writeDevInfoFile(le, outDistPath, devInfoFile, updatedInputMeta.GetDevInfo()); err != nil {
		return nil, errors.Wrap(err, "write updated dev info file")
	}

	le.Debug("fast rebuild complete")
	return updatedInputManifest, nil
}

// buildAndCheckoutEsbuildSubManifest builds the esbuild sub-manifest and checks out the results.
// It returns the web package references and esbuild output metadata extracted from the sub-manifest.
func (c *Controller) buildAndCheckoutEsbuildSubManifest(
	ctx context.Context,
	le *logrus.Entry,
	buildHost bldr_manifest_builder.BuildManifestHost,
	buildWorld world.Engine,
	pluginID string,
	sourcePath string,
	outAssetsPath string,
	esbuildBundleVarMeta []*EsbuildBundleVarMeta,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	baseEsbuildFlags []string,
) (
	webPkgRefs web_pkg.WebPkgRefSlice,
	esbuildOutputMeta []*bldr_web_bundler_esbuild.EsbuildOutputMeta,
	err error,
) {
	publicPath := BuildAssetHref(pluginID, EsbuildAssetSubdir)
	esbuildBundlerConf, err := BuildEsbuildBundlerConfig(esbuildBundleVarMeta, webPkgs, baseEsbuildFlags, sourcePath, publicPath)
	if err == nil {
		err = esbuildBundlerConf.Validate()
	}
	if err != nil {
		err = errors.Wrap(err, "failed to build esbuild bundler config")
		return
	}

	esbuildBuilderProto, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, esbuildBundlerConf), true)
	if err != nil {
		err = errors.Wrap(err, "failed to marshal esbuild bundler config")
		return
	}

	// build the manifest for this esbuild bundle
	subManifestID := "esbuild"
	le.Debug("waiting for esbuild sub-manifest")
	subManifestPromise, err := buildHost.BuildSubManifest(ctx, subManifestID, &bldr_project.ManifestConfig{
		Builder: esbuildBuilderProto,
	})
	if err != nil {
		err = errors.Wrap(err, "failed to start esbuild sub-manifest build")
		return
	}

	// wait for the result
	subManifestResult, err := subManifestPromise.Await(ctx)
	if err != nil {
		err = errors.Wrap(err, "esbuild sub-manifest build failed")
		return
	}

	// parse out the input manifest meta
	subManifestInput := subManifestResult.GetInputManifest()
	subManifestInputMeta := &bldr_web_bundler_esbuild_compiler.InputManifestMeta{}
	if err = subManifestInputMeta.UnmarshalVT(subManifestInput.GetMetadata()); err != nil {
		err = errors.Wrap(err, "unable to parse esbuild sub-manifest input metadata")
		return
	}

	// extract a couple variables we need later
	webPkgRefs = subManifestInputMeta.GetWebPkgRefs()
	esbuildOutputMeta = subManifestInputMeta.GetEsbuildOutputs()

	// sync the latest sub-manifest contents into our assets directory
	le.Debug("esbuild sub-manifest build complete, checking out assets")
	outAssetsEsbuildPath := filepath.Join(outAssetsPath, EsbuildAssetSubdir)
	_, err = bldr_manifest_world.CheckoutManifest(
		ctx,
		le,
		buildWorld.AccessWorldState,
		subManifestResult.GetManifestRef().GetManifestRef(),
		"", // No dist path for esbuild sub-manifest
		outAssetsEsbuildPath,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
	)
	if err != nil {
		err = errors.Wrap(err, "unable to extract esbuild sub-manifest")
		return
	}

	return webPkgRefs, esbuildOutputMeta, nil
}

// buildEsbuildGoVariableDefs generates the Go variable definitions based on esbuild outputs.
func buildEsbuildGoVariableDefs(
	pluginID string,
	esbuildBundleVarMeta []*EsbuildBundleVarMeta,
	esbuildOutputMeta []*bldr_web_bundler_esbuild.EsbuildOutputMeta,
) ([]*vardef.PluginVar, error) {
	var goVariableDefs []*vardef.PluginVar
	for _, bundleVarDef := range esbuildBundleVarMeta {
		// match each variable to a output entrypoint
		for _, entrypointVar := range bundleVarDef.GetEntrypointVars() {
			entrypointVarEsbuildEntrypointID := entrypointVar.ToEsbuildEntrypointId(bundleVarDef.GetId())

			// locate the esbuild output corresponding to this variable
			outputEntrypointIdx := slices.IndexFunc(esbuildOutputMeta, func(output *bldr_web_bundler_esbuild.EsbuildOutputMeta) bool {
				entrypointPath := output.GetPath()
				// TODO: is there a better way to determine the "actual" entrypoint (not the css bundle)?
				if !strings.HasSuffix(entrypointPath, ".mjs") && !strings.HasSuffix(entrypointPath, ".js") {
					return false
				}
				// match entrypoint id
				return output.GetEntrypointId() == entrypointVarEsbuildEntrypointID
			})
			if outputEntrypointIdx == -1 {
				return nil, errors.Errorf("could not find esbuild entrypoint corresponding to: %v", entrypointVarEsbuildEntrypointID)
			}
			outputEntrypoint := esbuildOutputMeta[outputEntrypointIdx]

			var outpEntrypointPath string
			if outpPath := outputEntrypoint.GetPath(); outpPath != "" {
				outpEntrypointPath = filepath.ToSlash(outpPath) // possibly unnecessary
				outpEntrypointPath = path.Join(EsbuildAssetSubdir, outpEntrypointPath)
			}

			var outpCssPath string
			if cssPath := outputEntrypoint.GetCssBundlePath(); cssPath != "" {
				outpCssPath = filepath.ToSlash(cssPath) // possibly unnecessary
				outpCssPath = path.Join(EsbuildAssetSubdir, outpCssPath)
			}

			// varValue is the value for the go variable.
			varType := entrypointVar.GetPkgVarType()
			pkgImportPath := entrypointVar.GetPkgImportPath()
			pkgVar := entrypointVar.GetPkgVar()
			var varDef *vardef.PluginVar
			switch varType {
			case EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH:
				var assetHref string
				if outpEntrypointPath != "" {
					assetHref = BuildAssetHref(pluginID, outpEntrypointPath)
				} else {
					assetHref = BuildAssetHref(pluginID, outpCssPath)
				}
				varDef = vardef.NewPluginVar(pkgImportPath, pkgVar, &vardef.PluginVar_StringValue{StringValue: assetHref})
			case EsbuildVarType_EsbuildVarType_WEB_BUNDLER_OUTPUT:
				output := &bldr_web_bundler.WebBundlerOutput{}
				if outpEntrypointPath != "" {
					output.EntrypointHref = BuildAssetHref(pluginID, outpEntrypointPath)
				}
				if outpCssPath != "" {
					output.CssHref = BuildAssetHref(pluginID, outpCssPath)
				}
				varDef = vardef.NewPluginVar(pkgImportPath, pkgVar, &vardef.PluginVar_WebBundlerOutput{
					WebBundlerOutput: output,
				})
			default:
				return nil, errors.Errorf("unknown target variable type: %s", varType.String())
			}

			goVariableDefs = append(goVariableDefs, varDef)
		}
	}
	return goVariableDefs, nil
}

// buildAndCheckoutViteSubManifest builds the vite sub-manifest and checks out the results.
// It returns the web package references and vite output metadata extracted from the sub-manifest.
func (c *Controller) buildAndCheckoutViteSubManifest(
	ctx context.Context,
	le *logrus.Entry,
	buildHost bldr_manifest_builder.BuildManifestHost,
	buildWorld world.Engine,
	outAssetsPath string,
	viteBundleVarMeta []*ViteBundleVarMeta,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	viteConfigPaths []string,
	disableProjectConfig bool,
) (
	webPkgRefs web_pkg.WebPkgRefSlice,
	viteOutputMeta []*bldr_vite.ViteOutputMeta,
	err error,
) {
	viteBundlerConf, err := BuildViteBundlerConfig(viteBundleVarMeta, webPkgs, viteConfigPaths, disableProjectConfig)
	if err == nil {
		err = viteBundlerConf.Validate()
	}
	if err != nil {
		err = errors.Wrap(err, "failed to build vite bundler config")
		return
	}

	viteBuilderProto, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, viteBundlerConf), true)
	if err != nil {
		err = errors.Wrap(err, "failed to marshal vite bundler config")
		return
	}

	// build the manifest for this vite bundle
	subManifestID := "vite"
	le.Debug("waiting for vite sub-manifest")
	subManifestPromise, err := buildHost.BuildSubManifest(ctx, subManifestID, &bldr_project.ManifestConfig{
		Builder: viteBuilderProto,
	})
	if err != nil {
		err = errors.Wrap(err, "failed to start vite sub-manifest build")
		return
	}

	// wait for the result
	subManifestResult, err := subManifestPromise.Await(ctx)
	if err != nil {
		err = errors.Wrap(err, "vite sub-manifest build failed")
		return
	}

	// parse out the input manifest meta
	subManifestInput := subManifestResult.GetInputManifest()
	subManifestInputMeta := &bldr_web_bundler_vite_compiler.InputManifestMeta{}
	if err = subManifestInputMeta.UnmarshalVT(subManifestInput.GetMetadata()); err != nil {
		err = errors.Wrap(err, "unable to parse vite sub-manifest input metadata")
		return
	}

	// extract a couple variables we need later
	webPkgRefs = subManifestInputMeta.GetWebPkgRefs()
	viteOutputMeta = subManifestInputMeta.GetViteOutputs()

	// sync the latest sub-manifest contents into our assets directory
	le.Debug("vite sub-manifest build complete, checking out assets")
	outAssetsVitePath := filepath.Join(outAssetsPath, ViteAssetSubdir)
	_, err = bldr_manifest_world.CheckoutManifest(
		ctx,
		le,
		buildWorld.AccessWorldState,
		subManifestResult.GetManifestRef().GetManifestRef(),
		"", // No dist path for vite sub-manifest
		outAssetsVitePath,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
	)
	if err != nil {
		err = errors.Wrap(err, "unable to extract vite sub-manifest")
		return
	}

	return webPkgRefs, viteOutputMeta, nil
}

// buildViteGoVariableDefs generates the Go variable definitions based on vite outputs.
func buildViteGoVariableDefs(
	pluginID string,
	viteBundleVarMeta []*ViteBundleVarMeta,
	viteOutputMeta []*bldr_vite.ViteOutputMeta,
) ([]*vardef.PluginVar, error) {
	var goVariableDefs []*vardef.PluginVar
	outputsByEntrypoint := make(map[string][]*bldr_vite.ViteOutputMeta)

	// Group output files by entrypoint
	for _, output := range viteOutputMeta {
		entrypointPath := output.GetEntrypointPath()
		if entrypointPath != "" {
			outputsByEntrypoint[entrypointPath] = append(outputsByEntrypoint[entrypointPath], output)
		}
	}

	// Process each bundle
	for _, bundleVarDef := range viteBundleVarMeta {
		// Match each variable to a output entrypoint
		for _, entrypointVar := range bundleVarDef.GetEntrypointVars() {
			// Build the full entrypoint path
			fullEntrypointPath := filepath.Join(entrypointVar.GetPkgCodePath(), entrypointVar.GetEntrypointPath())

			// Find all outputs for this entrypoint
			outputs, found := outputsByEntrypoint[fullEntrypointPath]
			if !found || len(outputs) == 0 {
				return nil, errors.Errorf(
					"no output found for vite entrypoint: %s.%s -> %s",
					entrypointVar.GetPkgImportPath(),
					entrypointVar.GetPkgVar(),
					fullEntrypointPath,
				)
			}

			// Find JS and CSS outputs
			var jsOutputPath, cssOutputPath string
			for _, output := range outputs {
				path := output.GetPath()
				ext := filepath.Ext(path)
				if ext == ".js" || ext == ".mjs" {
					jsOutputPath = path
				} else if ext == ".css" {
					cssOutputPath = path
				}
			}

			// Create variable based on type
			switch entrypointVar.GetPkgVarType() {
			case ViteVarType_ViteVarType_ENTRYPOINT_PATH:
				if jsOutputPath == "" {
					return nil, errors.Errorf(
						"no JS output found for vite entrypoint: %s.%s",
						entrypointVar.GetPkgImportPath(),
						entrypointVar.GetPkgVar(),
					)
				}

				// Build asset href and create string variable
				// Prepend ViteAssetSubdir to ensure the path is relative to the plugin assets root.
				jsAssetPath := path.Join(ViteAssetSubdir, jsOutputPath)
				assetHref := BuildAssetHref(pluginID, jsAssetPath)
				goVariableDefs = append(goVariableDefs, vardef.NewPluginVar(
					entrypointVar.GetPkgImportPath(),
					entrypointVar.GetPkgVar(),
					&vardef.PluginVar_StringValue{StringValue: assetHref},
				))

			case ViteVarType_ViteVarType_WEB_BUNDLER_OUTPUT:
				// Create WebBundlerOutput with JS and CSS references
				// Prepend ViteAssetSubdir to ensure the paths are relative to the plugin assets root.
				output := &bldr_web_bundler.WebBundlerOutput{}
				if jsOutputPath != "" {
					jsAssetPath := path.Join(ViteAssetSubdir, jsOutputPath)
					output.EntrypointHref = BuildAssetHref(pluginID, jsAssetPath)
				}
				if cssOutputPath != "" {
					cssAssetPath := path.Join(ViteAssetSubdir, cssOutputPath)
					output.CssHref = BuildAssetHref(pluginID, cssAssetPath)
				}

				goVariableDefs = append(goVariableDefs, vardef.NewPluginVar(
					entrypointVar.GetPkgImportPath(),
					entrypointVar.GetPkgVar(),
					&vardef.PluginVar_WebBundlerOutput{WebBundlerOutput: output},
				))
			}
		}
	}

	return goVariableDefs, nil
}

// writeDevInfoFile writes the plugin development info file if the path is specified.
func writeDevInfoFile(le *logrus.Entry, outDistPath, devInfoFile string, devInfo *vardef.PluginDevInfo) error {
	if devInfoFile == "" || devInfo == nil {
		return nil
	}

	devInfoBin, err := devInfo.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "failed to marshal dev info")
	}
	devInfoPath := filepath.Join(outDistPath, devInfoFile)
	if err := os.WriteFile(devInfoPath, devInfoBin, 0o644); err != nil {
		return errors.Wrapf(err, "failed to write dev info file %s", devInfoFile)
	}
	le.Debugf("wrote dev info file: %s", devInfoFile)
	return nil
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
