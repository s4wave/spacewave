//go:build !js

package bldr_web_bundler_esbuild_compiler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_bundler_esbuild "github.com/aperturerobotics/bldr/web/bundler/esbuild"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	bldr_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_vite "github.com/aperturerobotics/bldr/web/pkg/vite"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/world"
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
var controllerDescrip = "esbuild bundler controller"

// Inline sourcemaps due to Chrome bug
// https://issues.chromium.org/issues/40765087 [currently open 2024/03/25]
var inlineSourcemaps = true

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

// SupportsStartupManifestCache returns true if startup cache reuse is safe.
func (c *Controller) SupportsStartupManifestCache() bool {
	return true
}

// BuildManifest compiles the manifest with the given builder args.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, _, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// Override buildPlatform to the "none" platform since esbuild produces .js without the plugin wrapper.
	buildPlatform := bldr_platform.NewNonePlatform()
	meta.PlatformId = buildPlatform.GetPlatformID()

	platformID := meta.GetPlatformId()
	manifestID := strings.TrimSpace(meta.GetManifestId())
	sourcePath := builderConf.GetSourcePath()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	isRelease := buildType.IsRelease()

	// output paths
	workingPath := builderConf.GetWorkingPath()
	outDistPath := filepath.Join(workingPath, "dist")
	outAssetsPath := filepath.Join(workingPath, "assets")
	distSourcePath := builderConf.GetDistSourcePath()

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

	le := c.GetLogger().
		WithField("manifest-id", manifestID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building esbuild bundle")

	// If no web package files changed, rebuild esbuild assets only (hot reload)
	prevResult := args.GetPrevBuilderResult()
	var updatedManifestMeta *bldr_manifest_builder.InputManifest
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
			updatedManifestMeta, err = c.FastRebuildBundle(
				ctx,
				le,
				manifestID,
				sourcePath,
				distSourcePath,
				workingPath,
				outDistPath,
				outAssetsPath,
				prevResult.GetInputManifest(),
				args.GetChangedFiles(),
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
			res, err := hook(ctx, builderConf, busEngine)
			if err != nil {
				return nil, err
			}

			// merge the returned config
			buildCtrlConf.Merge(res.GetConfig())
		}

		baseEsbuildOpts, err := buildCtrlConf.ParseEsbuildFlags()
		if err != nil {
			return nil, err
		}

		// Process each bundle
		bundleList, err := BuildEsbuildBundleMeta(buildCtrlConf.GetBundles())
		if err != nil {
			return nil, err
		}

		// Build each bundle
		var webPkgRefs []*web_pkg.WebPkgRef
		var esbuildOutputMeta []*bldr_web_bundler_esbuild.EsbuildOutputMeta
		var sourceFilesList []string
		var esbuildBundles []*EsbuildBundleMeta

		for _, bundle := range bundleList {
			bundleID := bundle.GetId()
			publicPath := bundle.GetPublicPath()
			if publicPath == "" {
				publicPath = "/"
			}
			bundleWebPkgRefs, bundleOutputMeta, bundleSrcFiles, err := BuildEsbuildBundle(
				le,
				sourcePath,
				bundleID,
				bundle.GetEntrypoints(),
				baseEsbuildOpts,
				buildCtrlConf.GetWebPkgs(),
				outAssetsPath,
				publicPath,
				inlineSourcemaps,
				isRelease,
			)
			if err != nil {
				return nil, err
			}

			webPkgRefs = append(webPkgRefs, bundleWebPkgRefs...)
			esbuildOutputMeta = append(esbuildOutputMeta, bundleOutputMeta...)
			sourceFilesList = append(sourceFilesList, bundleSrcFiles...)
			esbuildBundles = append(esbuildBundles, bundle)
		}

		// Sort and deduplicate
		web_pkg.SortWebPkgRefs(webPkgRefs)
		esbuildOutputMeta = bldr_web_bundler_esbuild.SortEsbuildOutputMetas(esbuildOutputMeta)
		slices.Sort(sourceFilesList)
		sourceFilesList = slices.Compact(sourceFilesList)

		// build manifest metadata
		inputManifestMeta := &InputManifestMeta{
			WebPkgRefs:     webPkgRefs,
			WebPkgs:        buildCtrlConf.GetWebPkgs(),
			EsbuildBundles: esbuildBundles,
			EsbuildFlags:   buildCtrlConf.GetEsbuildFlags(),
			EsbuildOutputs: esbuildOutputMeta,
		}
		inputManifestMetaBin, err := inputManifestMeta.MarshalVT()
		if err != nil {
			return nil, err
		}

		updatedManifestMeta = &bldr_manifest_builder.InputManifest{Metadata: inputManifestMetaBin}
		inputFileKinds := map[InputFileKind][]string{
			InputFileKind_InputFileKind_ESBUILD: sourceFilesList,
		}

		// Build web pkgs with Vite (if any).
		// Filter out excluded web packages (another plugin provides these).
		excludedIDs := bldr_web_bundler.ExcludedWebPkgIDs(buildCtrlConf.GetWebPkgs())
		buildableWebPkgRefs := web_pkg.WebPkgRefSlice(webPkgRefs).FilterExcluded(excludedIDs)
		outWebPkgsPath := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
		var webPkgSrcFiles []string
		if len(buildableWebPkgRefs) != 0 {
			viteWorkingPath := filepath.Join(workingPath, "vite-web-pkgs")
			err = web_pkg_vite.RunOneShot(ctx, le, distSourcePath, sourcePath, viteWorkingPath, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
				_, srcFiles, _, buildErr := web_pkg_vite.BuildWebPkgsVite(
					ctx,
					le,
					sourcePath,
					buildableWebPkgRefs,
					outWebPkgsPath,
					bldr_plugin.PluginWebPkgHttpPrefix,
					isRelease,
					client,
					filepath.Join(viteWorkingPath, "cache"),
				)
				if buildErr == nil {
					webPkgSrcFiles = srcFiles
				}
				return buildErr
			})
			if err != nil {
				return nil, err
			}
		}
		inputFileKinds[InputFileKind_InputFileKind_WEB_PKG] = webPkgSrcFiles

		for kind, srcPaths := range inputFileKinds {
			meta := &InputFileMeta{Kind: kind}
			metaBin, err := meta.MarshalVT()
			if err != nil {
				return nil, err
			}

			err = fsutil.ConvertPathsToRelative(sourcePath, srcPaths)
			if err != nil {
				return nil, err
			}

			for _, srcPath := range srcPaths {
				updatedManifestMeta.Files = append(updatedManifestMeta.Files, &bldr_manifest_builder.InputManifest_File{
					Path:     srcPath,
					Metadata: metaBin,
				})
			}
		}
		updatedManifestMeta.SortFiles()
	}

	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	le.Debug("committing files to manifest")
	// bundle dist and assets fs
	committedManifest, committedManifestRef, err := builderConf.CommitManifestWithPaths(
		ctx,
		le,
		tx,
		meta,
		"", // no entrypoint
		outDistPath,
		outAssetsPath,
	)
	if err != nil {
		return nil, err
	}

	le.Debugf(
		"build complete with %d input files",
		len(updatedManifestMeta.Files),
	)
	result := bldr_manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		updatedManifestMeta,
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// FastRebuildBundle compiles the plugin once skipping running the web pkgs build process if possible.
// Assumes we are in dev mode (not release mode).
// Assumes the previous result is already checked out to outDistPath and outAssetsPath.
// Returns nil, nil if fast rebuild is not applicable.
func (c *Controller) FastRebuildBundle(
	ctx context.Context,
	le *logrus.Entry,
	manifestID,
	sourcePath,
	distSourcePath,
	workingPath,
	outDistPath,
	outAssetsPath string,
	prevInputManifest *bldr_manifest_builder.InputManifest,
	changedFiles []*bldr_manifest_builder.InputManifest_File,
) (*bldr_manifest_builder.InputManifest, error) {
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
		kind := meta.GetKind()
		if kind != InputFileKind_InputFileKind_ESBUILD {
			// Skip fast rebuild: non-esbuild asset
			return nil, nil
		}
	}

	// Perform fast rebuild by running the esbuild compiler only.
	le.Info("performing fast rebuild")

	// Process each bundle
	bundleList, err := BuildEsbuildBundleMeta(inputMeta.GetEsbuildBundles())
	if err != nil {
		return nil, err
	}

	// execute the build
	var updatedWebPkgRefs []*web_pkg.WebPkgRef
	var esbuildSrcFiles []string
	var updatedEsbuildOutputs []*bldr_web_bundler_esbuild.EsbuildOutputMeta
	for _, bundleDef := range bundleList {
		bundleID := bundleDef.GetId()
		publicPath := bundleDef.GetPublicPath()
		if publicPath == "" {
			publicPath = "/"
		}
		bundleWebPkgRefs, bundleOutputMeta, bundleSrcFiles, err := BuildEsbuildBundle(
			le,
			sourcePath,
			bundleID,
			bundleDef.GetEntrypoints(),
			baseEsbuildOpts,
			webPkgs,
			outAssetsPath,
			publicPath,
			inlineSourcemaps,
			false, // Not release mode for fast rebuild
		)
		if err != nil {
			return nil, err
		}

		esbuildSrcFiles = append(esbuildSrcFiles, bundleSrcFiles...)
		updatedEsbuildOutputs = append(updatedEsbuildOutputs, bundleOutputMeta...)
		for _, webPkgRef := range bundleWebPkgRefs {
			for _, impPath := range webPkgRef.Imports {
				updatedWebPkgRefs, _ = web_pkg.WebPkgRefSlice(updatedWebPkgRefs).AppendWebPkgRef(
					webPkgRef.WebPkgId,
					webPkgRef.WebPkgRoot,
					impPath,
				)
			}
		}
	}

	// cleanup esbuild src files list
	slices.Sort(esbuildSrcFiles)
	esbuildSrcFiles = slices.Compact(esbuildSrcFiles)

	// cleanup outputs list
	updatedEsbuildOutputs = bldr_web_bundler_esbuild.SortEsbuildOutputMetas(updatedEsbuildOutputs)

	// compare the outputs list with the old outputs list.
	// delete any output file from the old outputs that was not overwritten by esbuild.
	// for example: changed files with hashes in the filename will delete the old hash.
	updatedOutputs := make(map[string]struct{}, len(updatedEsbuildOutputs))
	for _, updatedOutput := range updatedEsbuildOutputs {
		updatedOutputs[updatedOutput.GetPath()] = struct{}{}
	}

	// Clean up old esbuild outputs
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

	// compare the web pkg refs to see if they changed.
	// if so: we must perform a full rebuild to pick up the new refs + rebuild the web pkgs.
	if !(&InputManifestMeta{WebPkgRefs: inputMeta.WebPkgRefs}).EqualVT(&InputManifestMeta{WebPkgRefs: updatedWebPkgRefs}) {
		le.Info("references to web pkgs changed: forcing a full re-build")
		return nil, nil
	}

	// build the updated input manifest
	updatedInputManifest := prevInputManifest.CloneVT()
	updatedInputMeta := inputMeta.CloneVT()
	updatedInputMeta.EsbuildOutputs = updatedEsbuildOutputs

	// drop all esbuild files from the set (we will add them back next)
	updatedInputManifest.Files = slices.DeleteFunc(updatedInputManifest.Files, func(f *bldr_manifest_builder.InputManifest_File) bool {
		meta.Reset()
		err := meta.UnmarshalVT(f.GetMetadata())
		if err != nil {
			return false
		}
		kind := meta.GetKind()
		return kind == InputFileKind_InputFileKind_ESBUILD
	})

	// add the updated esbuild files to the list
	if err := fsutil.ConvertPathsToRelative(sourcePath, esbuildSrcFiles); err != nil {
		return nil, err
	}
	esbuildFileMeta := &InputFileMeta{Kind: InputFileKind_InputFileKind_ESBUILD}
	esbuildFileMetaBin, err := esbuildFileMeta.MarshalVT()
	if err != nil {
		return nil, err
	}
	for _, srcPath := range esbuildSrcFiles {
		updatedInputManifest.Files = append(updatedInputManifest.Files, &bldr_manifest_builder.InputManifest_File{
			Path:     srcPath,
			Metadata: esbuildFileMetaBin,
		})
	}
	updatedInputManifest.SortFiles()

	// encode the updated meta
	updMeta, err := updatedInputMeta.MarshalVT()
	if err != nil {
		return nil, err
	}
	updatedInputManifest.Metadata = updMeta

	le.Debug("fast rebuild complete")
	return updatedInputManifest, nil
}

// GetSupportedPlatforms returns the base platform IDs this compiler supports.
// Returns nil because esbuild is a sub-manifest builder that produces platform-agnostic JS bundles.
func (c *Controller) GetSupportedPlatforms() []string {
	return nil
}

// _ is a type assertion
var _ bldr_manifest_builder.Controller = ((*Controller)(nil))
