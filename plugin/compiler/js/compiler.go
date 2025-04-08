package bldr_plugin_compiler_js

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_bundler_esbuild "github.com/aperturerobotics/bldr/web/bundler/esbuild"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	bldr_web_bundler_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	bldr_web_bundler_esbuild_compiler "github.com/aperturerobotics/bldr/web/bundler/esbuild/compiler"
	bldr_web_bundler_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/aperturerobotics/bldr/web/bundler/vite/compiler"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	protobuf_go_lite_json "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
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
	builderConf *manifest_builder.BuilderConfig,
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
	args *manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, _, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// Override buildPlatform to the "js" platform
	buildPlatform := bldr_platform.NewWebPlatformJs()
	meta.PlatformId = buildPlatform.GetPlatformID()

	platformID := meta.GetPlatformId()
	manifestID := strings.TrimSpace(meta.GetManifestId())
	// sourcePath := builderConf.GetSourcePath()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	isRelease := buildType.IsRelease()

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

	le := c.GetLogger().
		WithField("manifest-id", manifestID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building js plugin")

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

	// call any pre-build hooks
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, buildWorld)
		if err != nil {
			return nil, err
		}

		// merge the returned config
		buildCtrlConf.Merge(res.GetConfig())
	}

	webPkgs := buildCtrlConf.GetWebPkgs() // pass to bundlers if needed

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
	// If bundles with the same name ("backend" or "frontend") are already defined
	// in vite_bundles, they will be merged later during the Vite compiler build step.
	for _, mod := range buildCtrlConf.GetModules() {
		inputPath := path.Clean(mod.GetPath())
		ext := filepath.Ext(inputPath)

		// assume the output path corresponding to inputPath is InputPath sub extension plus .mjs
		// add the b/vite prefix
		outputPath := path.Join(
			bldr_plugin_compiler.ViteAssetSubdir,
			strings.TrimSuffix(inputPath, ext)+buildPlatform.GetExecutableExt(),
		)

		// configure the bundle type
		var bundleID string
		modKind := mod.GetKind()
		switch modKind {
		case JsModuleKind_JS_MODULE_KIND_BACKEND:
			// vite bundle id
			bundleID = "backend"

			// set entrypoint if enabled
			if !mod.GetDisableEntrypoint() {
				backendEntrypoints = append(backendEntrypoints, &BackendEntrypoint{
					ImportPath: outputPath,
				})
			}
		case JsModuleKind_JS_MODULE_KIND_FRONTEND:
			// vite bundle id
			bundleID = "frontend"

			// set entrypoint if enabled
			if !mod.GetDisableEntrypoint() {
				frontendEntrypoints = append(frontendEntrypoints, &FrontendEntrypoint{
					ImportPath: outputPath,
				})
			}
		default:
			return nil, errors.Errorf("unknown js module kind: %s", modKind.String())
		}

		// add a bundle for this module
		vb := &bldr_web_bundler_vite_compiler.ViteBundleMeta{
			Id: bundleID,
			Entrypoints: []*bldr_web_bundler_vite_compiler.ViteBundleEntrypoint{{
				InputPath: inputPath,
			}},
			ViteConfigPaths:      mod.GetViteConfigPaths(),
			DisableProjectConfig: mod.GetDisableProjectConfig(),
		}
		buildCtrlConf.ViteBundles = append(buildCtrlConf.ViteBundles, vb)
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
	}

	// Sort collected web package references
	web_pkg.SortWebPkgRefs(allWebPkgRefs)

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

	defines := map[string]string{
		// Pass JSON array strings to esbuild define
		"__BLDR_BACKEND_ENTRYPOINTS__":  backendEpJsonStr,
		"__BLDR_FRONTEND_ENTRYPOINTS__": frontendEpJsonStr,
		"__BLDR_HOST_CONFIG_SET__":      hostConfigSetJsonStr,
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

	// Run esbuild
	result := esbuild_api.Build(buildOptions)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return nil, errors.Wrap(err, "failed to compile js plugin entrypoint")
	}

	// Parse the metafile to find the actual output path for our entrypoint
	metafileData := &bldr_web_bundler_esbuild_build.EsbuildMetafile{}
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
		WebPkgs:    bldr_web_bundler.CompactWebPkgRefConfigs(slices.Clone(webPkgs)),

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

	// Create the InputManifest object (no input files tracked for JS compiler currently)
	inputManifest := manifest_builder.NewInputManifest(nil, inputManifestMetaBin)

	// -- Commit assets to the manifest store --
	tx, err := buildWorld.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	le.Debug("committing assets to manifest")
	// Commit manifest with assets.
	// Dist path and entrypoint name are empty for JS plugins, as the primary
	// entrypoint is the compiled asset specified in InputManifestMeta.
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
	builderResult := manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		inputManifest, // Include the input manifest with metadata
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return builderResult, nil
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
