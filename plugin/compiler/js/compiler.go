//go:build !js

package bldr_plugin_compiler_js

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_bundler_esbuild "github.com/aperturerobotics/bldr/web/bundler/esbuild"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	bldr_web_bundler_esbuild_compiler "github.com/aperturerobotics/bldr/web/bundler/esbuild/compiler"
	bldr_web_bundler_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/aperturerobotics/bldr/web/bundler/vite/compiler"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"
	"github.com/aperturerobotics/bldr/util/npm"
	bldr_web_plugin "github.com/aperturerobotics/bldr/web/plugin"
	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	protobuf_go_lite_json "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "js plugin compiler controller"

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
	builderConf *bldr_manifest_builder.BuilderConfig,
	worldEng world.Engine,
) (*PreBuildHookResult, error)

// AddPreBuildHook adds a callback that is called just after constructing the plugin working dir.
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
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	pluginID := meta.GetManifestId()
	platformID := meta.GetPlatformId()
	manifestID := strings.TrimSpace(meta.GetManifestId())
	// sourcePath := builderConf.GetSourcePath()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	isRelease := buildType.IsRelease()

	le := c.GetLogger().
		WithField("manifest-id", manifestID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)

	// Do nothing if we are not targeting a supported platform.
	if buildPlatform.GetBasePlatformID() != bldr_platform.PlatformID_JS {
		le.Warnf("skipping build for non-js platform: %v", buildPlatform.GetInputPlatformID())
		return nil, nil
	}
	le.Debug("building js plugin")

	// output paths, dist is unused for JS compiler
	workingPath := builderConf.GetWorkingPath()
	// Note: outDistPath is not typically used by the JS compiler itself,
	// but we create it for consistency and potential future use.
	outDistPath := filepath.Join(workingPath, "dist")
	outAssetsPath := filepath.Join(workingPath, "assets")
	// distSourcePath is used to locate the entrypoint.ts template.
	distSourcePath := builderConf.GetDistSourcePath()

	// build output world engine
	buildWorld := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

	// create build directories if they don't exist
	if err := fsutil.CreateDir(outDistPath); err != nil {
		return nil, err
	}
	if err := fsutil.CreateDir(outAssetsPath); err != nil {
		return nil, err
	}

	// Check out the previous result if any to save time.
	// Note: JS compiler doesn't have a fast-rebuild path like Go compiler yet.
	prevResult := args.GetPrevBuilderResult()
	if !prevResult.GetManifestRef().GetEmpty() && !isRelease {
		prevManifestRef := prevResult.GetManifestRef()
		_, err = builderConf.CheckoutManifest(
			ctx,
			le,
			buildWorld.AccessWorldState,
			prevManifestRef.GetManifestRef(),
			"", // no dist path
			outAssetsPath,
		)
		if err != nil {
			// Log warning but continue, as checkout failure isn't fatal for a full build.
			le.WithError(err).Warn("failed to check out previous manifest assets")
		}
	}

	// build base config
	buildCtrlConf := conf.CloneVT()
	if buildCtrlConf == nil {
		buildCtrlConf = &Config{}
	}

	// apply the per-build-type configs
	buildCtrlConf.FlattenBuildTypes(buildType)

	// apply the per-platform-type configs
	buildCtrlConf.FlattenPlatformTypes(buildPlatform)

	// call any pre-build hooks
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, buildWorld)
		if err != nil {
			return nil, err
		}

		// merge the returned config
		buildCtrlConf.Merge(res.GetConfig())
	}

	// Compact web packages list
	webPkgs := bldr_web_bundler.CompactWebPkgRefConfigs(slices.Clone(buildCtrlConf.GetWebPkgs()))

	// Esbuild configuration
	esbuildBundleMetas := buildCtrlConf.GetEsbuildBundles()
	baseEsbuildFlags := buildCtrlConf.GetEsbuildFlags() // Use raw flags

	// Prepare backend and frontend entrypoints from the config.
	// We clone the slices to avoid modifying the original config object directly,
	// although buildCtrlConf itself is already a clone.
	backendEntrypoints := slices.Clone(buildCtrlConf.GetBackendEntrypoints())
	frontendEntrypoints := slices.Clone(buildCtrlConf.GetFrontendEntrypoints())

	// Configure bundles and potentially add default entrypoints based on jsModules.
	// This adds default Vite bundles for modules defined with the shortcut syntax.
	// If bundles with the same name ("fe" or "be") are already defined in vite_bundles,
	// they will be merged later during the Vite compiler build step.
	for _, mod := range buildCtrlConf.GetModules() {
		// on the frontend, pass BldrExternal as external packages.
		var externalPkgs []string

		// configure the bundle type
		var bundleID string
		modKind := mod.GetKind()
		switch modKind {
		case JsModuleKind_JS_MODULE_KIND_BACKEND:
			// vite bundle id
			bundleID = "be"
		case JsModuleKind_JS_MODULE_KIND_FRONTEND:
			// vite bundle id
			bundleID = "fe"

			// external pkgs
			externalPkgs = web_pkg_external.BldrExternal
		default:
			return nil, errors.Errorf("unknown js module kind: %s", modKind.String())
		}

		// add a bundle for this module
		inputPath := path.Clean(mod.GetPath())
		buildCtrlConf.ViteBundles = append(buildCtrlConf.ViteBundles, &bldr_web_bundler_vite_compiler.ViteBundleMeta{
			Id: bundleID,
			Entrypoints: []*bldr_web_bundler_vite_compiler.ViteBundleEntrypoint{{
				InputPath: inputPath,
			}},
			ViteConfigPaths:      mod.GetViteConfigPaths(),
			DisableProjectConfig: mod.GetDisableProjectConfig(),

			// TODO: is there a way we can set this dynamically at runtime?
			// TODO: if the plugin ID changes this URL will change.
			PublicPath: bldr_plugin.PluginAssetHTTPPath(pluginID, path.Join("v", "b", bundleID)),

			ExternalPkgs: externalPkgs,
		})
	}

	// Vite configuration
	viteBundleMetas := buildCtrlConf.GetViteBundles()
	baseViteConfPaths := buildCtrlConf.GetViteConfigPaths()
	viteDisableProjectConfig := buildCtrlConf.GetViteDisableProjectConfig()

	// Collect web package references from bundlers
	var allWebPkgRefs web_pkg.WebPkgRefSlice

	// Store output metadata from bundlers
	var esbuildOutputMeta []*bldr_web_bundler_esbuild.EsbuildOutputMeta
	var viteOutputMeta []*bldr_web_bundler_vite.ViteOutputMeta

	// Build Esbuild bundles if configured
	if len(esbuildBundleMetas) != 0 {
		le.Info("building esbuild bundles")
		esbuildBundlerConf := &bldr_web_bundler_esbuild_compiler.Config{
			Bundles:      esbuildBundleMetas,
			WebPkgs:      webPkgs,
			EsbuildFlags: baseEsbuildFlags,
			// PublicPath is not needed here as it's handled by the Go compiler variable injection
		}
		if err := esbuildBundlerConf.Validate(); err != nil {
			return nil, errors.Wrap(err, "invalid esbuild bundler config")
		}

		esbuildBuilderProto, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, esbuildBundlerConf), true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal esbuild bundler config")
		}

		esbuildWebPkgRefs, esbuildOutMeta, err := bldr_plugin_compiler.BuildAndCheckoutEsbuildSubManifest(
			ctx,
			le,
			host,
			buildWorld,
			outAssetsPath,
			esbuildBuilderProto,
		)
		if err != nil {
			return nil, err
		}
		esbuildOutputMeta = esbuildOutMeta
		allWebPkgRefs = append(allWebPkgRefs, esbuildWebPkgRefs...)
	}

	// Build Vite bundles if configured
	if len(viteBundleMetas) != 0 {
		le.Info("building vite bundles")
		viteBundlerConf := &bldr_web_bundler_vite_compiler.Config{
			Bundles:              viteBundleMetas,
			WebPkgs:              webPkgs,
			ViteConfigPaths:      baseViteConfPaths,
			DisableProjectConfig: viteDisableProjectConfig,
		}
		if err := viteBundlerConf.Validate(); err != nil {
			return nil, errors.Wrap(err, "invalid vite bundler config")
		}

		viteBuilderProto, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, viteBundlerConf), true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal vite bundler config")
		}

		viteWebPkgRefs, viteOutMeta, err := bldr_plugin_compiler.BuildAndCheckoutViteSubManifest(
			ctx,
			le,
			host,
			buildWorld,
			outAssetsPath,
			viteBuilderProto,
		)
		if err != nil {
			return nil, err
		}
		viteOutputMeta = viteOutMeta
		allWebPkgRefs = append(allWebPkgRefs, viteWebPkgRefs...)

		// Match outputs to the input modules and create entrypoints with actual hashed paths
		backendEntrypoints, frontendEntrypoints = CreateEntrypointsFromViteOutputs(
			buildCtrlConf.GetModules(),
			viteOutputMeta,
			backendEntrypoints,
			frontendEntrypoints,
		)
	}

	// Filter out excluded web package references (another plugin provides these).
	excludedIDs := bldr_web_bundler.ExcludedWebPkgIDs(webPkgs)
	allWebPkgRefs = allWebPkgRefs.FilterExcluded(excludedIDs)

	// Sort collected web package references
	web_pkg.SortWebPkgRefs(allWebPkgRefs)

	// Install dist deps for the entrypoint build (cached: skips if package.json unchanged).
	// The entrypoint bundles @aptre/bldr which transitively imports packages
	// (like workbox-window) that must be resolved via dist/deps/package.json.
	distDepsDir := filepath.Join(workingPath, "dist-deps")
	if err := npm.EnsureBunInstall(ctx, le, workingPath, filepath.Join(distSourcePath, "dist/deps/package.json"), distDepsDir); err != nil {
		return nil, errors.Wrap(err, "failed to install dist deps for entrypoint")
	}
	distDepsNodeModules := filepath.Join(distDepsDir, "node_modules")

	// -- Compile the main JS entrypoint (plugin-{hash}.mjs) --
	le.Info("compiling js plugin entrypoint")
	entrypointTsSrcPath := filepath.Join(distSourcePath, "plugin", "compiler", "js", "entrypoint.ts")

	// Verify entrypoint source exists
	if _, err := os.Stat(entrypointTsSrcPath); err != nil {
		return nil, errors.Wrapf(err, "js plugin entrypoint source: %s", entrypointTsSrcPath)
	}

	// Marshal backend entrypoints to JSON array string
	backendEpJsonBytes, err := protobuf_go_lite_json.MarshalSlice(protobuf_go_lite_json.DefaultMarshalerConfig, backendEntrypoints)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal backend entrypoints")
	}
	backendEpJsonStr := string(backendEpJsonBytes)

	// Marshal frontend entrypoints to JSON array string
	frontendEpJsonBytes, err := protobuf_go_lite_json.MarshalSlice(protobuf_go_lite_json.DefaultMarshalerConfig, frontendEntrypoints)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal frontend entrypoints")
	}
	frontendEpJsonStr := string(frontendEpJsonBytes)

	// Marshal host config set to JSON array string.
	hostConfigSet := buildCtrlConf.GetHostConfigSet()
	hostConfigSetJsonStr := "undefined"
	if len(hostConfigSet) != 0 {
		hostConfigSetJson, err := protobuf_go_lite_json.MarshalMap(protobuf_go_lite_json.DefaultMarshalerConfig, hostConfigSet)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal host config set")
		}
		hostConfigSetJsonStr = string(hostConfigSetJson)
	}

	// Marshal web pkgs configuration to JSON
	handleWebPkgsJsonStr := "undefined"
	webPkgIds := allWebPkgRefs.ToWebPkgIDList()
	if len(webPkgIds) != 0 {
		// HandlePluginId is filled in at runtime.
		handleWebPkgs := &bldr_web_plugin.HandleWebPkgsViaPluginAssetsRequest{
			WebPkgsPath:  bldr_plugin.PluginAssetsWebPkgsDir,
			WebPkgIdList: webPkgIds,
		}
		handleWebPkgsJson, err := handleWebPkgs.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal handle web pkgs request")
		}
		handleWebPkgsJsonStr = string(handleWebPkgsJson)
	}

	defines := map[string]string{
		// Pass JSON array strings to esbuild define
		"__BLDR_BACKEND_ENTRYPOINTS__":  backendEpJsonStr,
		"__BLDR_FRONTEND_ENTRYPOINTS__": frontendEpJsonStr,
		"__BLDR_HOST_CONFIG_SET__":      hostConfigSetJsonStr,
		"__BLDR_HANDLE_WEB_PKGS__":      handleWebPkgsJsonStr,
		"__BLDR_WEB_PLUGIN_ID__":        strconv.Quote(conf.GetWebPluginId()),
	}

	// Relative path to the entrypoint within the distSourcePath directory.
	entrypointTsRelativePath := "plugin/compiler/js/entrypoint.ts"

	// Desired output path structure (esbuild will add hash and extension)
	entrypointOutputBase := "plugin" // plugin-HASH.mjs

	// Configure esbuild options for the plugin entrypoint
	buildOptions := entrypoint_browser_bundle.BrowserBuildOpts(distSourcePath, isRelease)

	// Override/set specific fields for this entrypoint build.
	buildOptions.Outdir = outDistPath         // Write assets to the output directory.
	buildOptions.EntryNames = "plugin-[hash]" // Use hashed filenames for cache busting.
	buildOptions.EntryPoints = nil            // Clear any default entrypoints from BrowserBuildOpts.
	buildOptions.EntryPointsAdvanced = []esbuild_api.EntryPoint{
		{
			InputPath:  entrypointTsRelativePath,
			OutputPath: entrypointOutputBase, // Define the output structure (name part, hash added by EntryNames).
		},
	}
	buildOptions.Define = defines                        // Inject backend/frontend entrypoint paths.
	buildOptions.Metafile = true                         // Enable metafile to find the hashed output path.
	buildOptions.Splitting = false                       // Do not split code for this simple entrypoint.
	buildOptions.Sourcemap = esbuild_api.SourceMapInline // Inline sourcemap for easier debugging.
	buildOptions.Write = true
	buildOptions.NodePaths = []string{distDepsNodeModules}

	buildOptions.Plugins = append(buildOptions.Plugins,
		bldr_esbuild_build.GoVendorTsResolverPlugin(builderConf.GetSourcePath()),
	)

	// Run esbuild
	result := esbuild_api.Build(buildOptions)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return nil, errors.Wrap(err, "failed to compile js plugin entrypoint")
	}

	// Parse the metafile to find the actual output path for our entrypoint
	metafileData := &bldr_esbuild_build.EsbuildMetafile{}
	if err := json.Unmarshal([]byte(result.Metafile), metafileData); err != nil {
		return nil, errors.Wrap(err, "failed to parse esbuild metafile")
	}

	// Find the output corresponding to the entrypoint specified in EntryPointsAdvanced.
	// The key in the metafileData.Outputs map is the path relative to the Outdir (outAssetsPath).
	// The EntryPoint in the value should match the InputPath we provided.
	entrypointOutputPath := ""
	for outPath, outMeta := range metafileData.Outputs {
		if outMeta.EntryPoint == entrypointTsRelativePath {
			entrypointOutputPath = outPath
			break
		}
	}
	if entrypointOutputPath == "" {
		return nil, errors.Errorf("unable to find output path for entrypoint %s in esbuild metafile", entrypointTsRelativePath)
	}

	// The path in the metafile is relative to distSourceDir
	compiledEntrypointRelPath, err := filepath.Rel(outDistPath, filepath.Join(distSourcePath, entrypointOutputPath))
	if err != nil {
		return nil, err
	}
	compiledEntrypointRelPath = path.Clean(filepath.ToSlash(compiledEntrypointRelPath))

	le.Debugf("compiled js plugin entrypoint to %s", compiledEntrypointRelPath)

	// Build final input manifest metadata
	inputManifestMeta := &InputManifestMeta{
		WebPkgRefs: allWebPkgRefs,
		WebPkgs:    webPkgs,

		EsbuildBundles: esbuildBundleMetas,
		EsbuildFlags:   baseEsbuildFlags,
		EsbuildOutputs: esbuildOutputMeta,

		ViteBundles:              viteBundleMetas,
		ViteConfigPaths:          baseViteConfPaths,
		ViteOutputs:              viteOutputMeta,
		ViteDisableProjectConfig: viteDisableProjectConfig,

		CompiledEntrypointPath: compiledEntrypointRelPath,
	}
	inputManifestMetaBin, err := inputManifestMeta.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal input manifest metadata")
	}

	// Create the InputManifest object
	//
	// NOTE: All input files are tracked by the sub-manifest system.
	inputManifest := bldr_manifest_builder.NewInputManifest(nil, inputManifestMetaBin)

	// -- Commit assets to the manifest store --
	tx, err := buildWorld.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Commit manifest with assets.
	// Dist path and entrypoint name are empty for JS plugins, as the primary
	// entrypoint is the compiled asset specified in InputManifestMeta.
	le.Debug("committing assets to manifest")
	committedManifest, committedManifestRef, err := builderConf.CommitManifestWithPaths(
		ctx,
		le,
		tx,
		meta,
		compiledEntrypointRelPath,
		outDistPath,
		outAssetsPath,
	)
	if err != nil {
		return nil, err
	}

	// -- Finalize and return result --
	le.Debug("js plugin build complete")
	builderResult := bldr_manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		inputManifest, // Include the input manifest with metadata
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return builderResult, nil
}

// CreateEntrypointsFromViteOutputs matches Vite outputs to JS modules and creates backend/frontend entrypoints.
// Returns the updated backend and frontend entrypoint slices.
func CreateEntrypointsFromViteOutputs(
	modules []*JsModule,
	viteOutputMeta []*bldr_web_bundler_vite.ViteOutputMeta,
	existingBackendEntrypoints []*BackendEntrypoint,
	existingFrontendEntrypoints []*FrontendEntrypoint,
) ([]*BackendEntrypoint, []*FrontendEntrypoint) {
	backendEntrypoints := slices.Clone(existingBackendEntrypoints)
	frontendEntrypoints := slices.Clone(existingFrontendEntrypoints)

	for _, mod := range modules {
		inputPath := path.Clean(mod.GetPath())
		modKind := mod.GetKind()

		// Skip if entrypoint is disabled
		if mod.GetDisableEntrypoint() {
			continue
		}

		// Find the corresponding Vite output for this module
		var jsOutputPath string
		var cssOutputPaths []string

		for _, output := range viteOutputMeta {
			matchesEntrypoint := output.GetEntrypointPath() != "" && output.GetEntrypointPath() == inputPath
			outputPath := output.GetPath()
			if strings.HasSuffix(outputPath, ".mjs") || strings.HasSuffix(outputPath, ".js") {
				if matchesEntrypoint {
					jsOutputPath = path.Join(bldr_plugin_compiler.ViteAssetSubdir, outputPath)
				}
			} else if strings.HasSuffix(outputPath, ".css") {
				// empty entrypointPath = global css files
				if matchesEntrypoint || output.GetEntrypointPath() == "" {
					cssOutputPaths = append(cssOutputPaths, path.Join(bldr_plugin_compiler.ViteAssetSubdir, outputPath))
				}
			}
		}

		// Add entrypoints based on module kind
		switch modKind {
		case JsModuleKind_JS_MODULE_KIND_BACKEND:
			if jsOutputPath == "" {
				break
			}
			backendEntrypoints = append(backendEntrypoints, &BackendEntrypoint{
				ImportPath: path.Join("/assets", jsOutputPath),
			})
		case JsModuleKind_JS_MODULE_KIND_FRONTEND:
			if jsOutputPath == "" {
				break
			}

			// Create frontend entrypoint
			frontendEp := &FrontendEntrypoint{
				SetRenderMode: &web_view.SetRenderModeRequest{
					RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
					ScriptPath: jsOutputPath,
					Refresh:    true,
				},
			}

			// Add CSS links if any
			if len(cssOutputPaths) != 0 {
				frontendEp.SetHtmlLinks = &web_view.SetHtmlLinksRequest{
					Clear:    true,
					SetLinks: make(map[string]*web_view.HtmlLink),
				}

				for _, cssPath := range cssOutputPaths {
					linkKey := "css-" + path.Base(cssPath)
					frontendEp.SetHtmlLinks.SetLinks[linkKey] = &web_view.HtmlLink{
						Rel:  "stylesheet",
						Href: cssPath,
					}
				}
			}

			frontendEntrypoints = append(frontendEntrypoints, frontendEp)
		}
	}

	return backendEntrypoints, frontendEntrypoints
}

// GetSupportedPlatforms returns the base platform IDs this compiler supports.
func (c *Controller) GetSupportedPlatforms() []string {
	return []string{bldr_platform.PlatformID_JS}
}

// _ is a type assertion
var _ bldr_manifest_builder.Controller = ((*Controller)(nil))
