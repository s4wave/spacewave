//go:build !js

package bldr_web_bundler_vite_compiler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_bundler_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "vite bundler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
	preBuildHooks []PreBuildHook

	// bundlerRc is the bundler refcount instance.
	viteBundlers *keyed.KeyedRefCount[viteBundlerKey, *viteBundlerTracker]
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewControllerWithBusController constructs a new plugin compiler controller with an existing BusController.
func NewControllerWithBusController(base *bus.BusController[*Config]) (*Controller, error) {
	c := &Controller{
		BusController: base,
	}

	c.viteBundlers = keyed.NewKeyedRefCount(
		c.buildViteCompilerTracker,
		keyed.WithExitLoggerWithNameFn[viteBundlerKey, *viteBundlerTracker](c.GetLogger(), func(key viteBundlerKey) string { return "bundle-" + key.bundleID }),
		keyed.WithReleaseDelay[viteBundlerKey, *viteBundlerTracker](time.Second*30),
		keyed.WithRetry[viteBundlerKey, *viteBundlerTracker](&backoff.Backoff{}),
	)

	return c, nil
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
	c.viteBundlers.SetContext(ctx, true)
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
	meta, _, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// Override buildPlatform to the "none" platform since vite produces .js without the plugin wrapper.
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
	le.Debug("building vite bundle")

	// Try fast rebuild first if we have a previous result and not in release mode
	var updatedManifestMeta *bldr_manifest_builder.InputManifest
	prevResult := args.GetPrevBuilderResult()
	if !prevResult.GetManifestRef().GetEmpty() && !isRelease {
		var err error
		updatedManifestMeta, err = c.tryFastRebuild(
			ctx,
			le,
			builderConf,
			busEngine,
			manifestID,
			sourcePath,
			distSourcePath,
			workingPath,
			outDistPath,
			outAssetsPath,
			prevResult,
			args.GetChangedFiles(),
		)
		if err != nil {
			le.WithError(err).Warn("fast rebuild failed: continuing with normal build")
			updatedManifestMeta = nil
		} else if updatedManifestMeta != nil {
			le.Debug("completed fast rebuild")
		}
	}

	// If fast rebuild was skipped or failed, perform a full rebuild
	if updatedManifestMeta == nil {
		var err error
		updatedManifestMeta, err = c.performFullRebuild(
			ctx,
			le,
			conf,
			builderConf,
			busEngine,
			manifestID,
			sourcePath,
			distSourcePath,
			workingPath,
			outDistPath,
			outAssetsPath,
			buildType,
			isRelease,
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

// viteBuildResult stores the results of building Vite bundles.
type viteBuildResult struct {
	webPkgRefs   []*web_pkg.WebPkgRef
	viteOutputs  []*bldr_web_bundler_vite.ViteOutputMeta
	viteSrcFiles []string
	viteBundles  []*ViteBundleMeta
}

// bundleBuildResult holds the result of building a single bundle.
type bundleBuildResult struct {
	webPkgRefs []*web_pkg.WebPkgRef
	outputMeta []*bldr_web_bundler_vite.ViteOutputMeta
	srcFiles   []string
	bundleMeta *ViteBundleMeta
}

// buildViteBundles builds all Vite bundles concurrently and returns the aggregated results.
func (c *Controller) buildViteBundles(
	ctx context.Context,
	le *logrus.Entry,
	distSourcePath,
	sourcePath,
	workingPath string,
	viteConfigPaths []string,
	bundleList []*ViteBundleMeta,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	outAssetsPath,
	manifestID string,
	isRelease bool,
) (*viteBuildResult, error) {
	if len(bundleList) == 0 {
		return &viteBuildResult{}, nil
	}

	// Build bundles concurrently using errgroup.
	eg, egCtx := errgroup.WithContext(ctx)
	results := make([]*bundleBuildResult, len(bundleList))
	var mu sync.Mutex

	for i, bundle := range bundleList {
		eg.Go(func() error {
			bundleID := bundle.Id
			key := newViteBundlerKey(
				distSourcePath,
				sourcePath,
				workingPath,
				bundleID,
			)

			var bundleWebPkgRefs []*web_pkg.WebPkgRef
			var bundleOutputMeta []*bldr_web_bundler_vite.ViteOutputMeta
			var bundleSrcFiles []string
			var err error

			// Retry up to 3 times if we get "stream reset" errors.
			for attempt := range 3 {
				ref, bundlerTkr, _ := c.viteBundlers.AddKeyRef(key)
				bundler, awaitErr := bundlerTkr.instancePromiseCtr.Await(egCtx)
				if awaitErr != nil {
					ref.Release()
					return awaitErr
				}

				bundleWebPkgRefs, bundleOutputMeta, bundleSrcFiles, err = BuildViteBundle(
					egCtx,
					le.WithField("bundle", bundleID),
					distSourcePath,
					sourcePath,
					workingPath,
					viteConfigPaths,
					bundle,
					bundler,
					webPkgs,
					outAssetsPath,
					manifestID,
					isRelease,
				)
				ref.Release()

				if err == nil {
					break
				}

				if strings.HasSuffix(err.Error(), "stream reset") {
					le.WithField("bundle", bundleID).WithField("attempt", attempt+1).Warn("restarting vite: got stream reset error")
					mu.Lock()
					_, _ = c.viteBundlers.RestartRoutine(key)
					mu.Unlock()
					continue
				}

				return err
			}

			if err != nil {
				return err
			}

			results[i] = &bundleBuildResult{
				webPkgRefs: bundleWebPkgRefs,
				outputMeta: bundleOutputMeta,
				srcFiles:   bundleSrcFiles,
				bundleMeta: bundle,
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Aggregate results.
	var webPkgRefs []*web_pkg.WebPkgRef
	var viteOutputMeta []*bldr_web_bundler_vite.ViteOutputMeta
	var sourceFilesList []string
	var viteBundles []*ViteBundleMeta

	for _, r := range results {
		if r != nil {
			webPkgRefs = append(webPkgRefs, r.webPkgRefs...)
			viteOutputMeta = append(viteOutputMeta, r.outputMeta...)
			sourceFilesList = append(sourceFilesList, r.srcFiles...)
			viteBundles = append(viteBundles, r.bundleMeta)
		}
	}

	// Sort and deduplicate.
	web_pkg.SortWebPkgRefs(webPkgRefs)
	viteOutputMeta = bldr_web_bundler_vite.SortViteOutputMetas(viteOutputMeta)
	slices.Sort(sourceFilesList)
	sourceFilesList = slices.Compact(sourceFilesList)

	return &viteBuildResult{
		webPkgRefs:   webPkgRefs,
		viteOutputs:  viteOutputMeta,
		viteSrcFiles: sourceFilesList,
		viteBundles:  viteBundles,
	}, nil
}

// cleanupOldViteOutputs removes old Vite output files that are no longer needed.
func (c *Controller) cleanupOldViteOutputs(
	le *logrus.Entry,
	outAssetsPath string,
	oldOutputs []*bldr_web_bundler_vite.ViteOutputMeta,
	newOutputs []*bldr_web_bundler_vite.ViteOutputMeta,
) error {
	newOutputPaths := make(map[string]struct{}, len(newOutputs))
	for _, output := range newOutputs {
		newOutputPaths[output.GetPath()] = struct{}{}
	}

	for _, oldOutput := range oldOutputs {
		if _, exists := newOutputPaths[oldOutput.GetPath()]; !exists {
			oldOutputPath := oldOutput.GetPath()
			absPath := filepath.Join(outAssetsPath, oldOutputPath)
			relPath, err := filepath.Rel(outAssetsPath, absPath)
			if err != nil {
				return err
			}
			if strings.HasPrefix(relPath, "..") {
				// prevent deleting things outside the assets dir
				le.Warnf("skipping removing old output path outside assets dir: %s", relPath)
				continue
			}
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				le.Warnf("old output path not found: %s", oldOutputPath)
			} else if err := os.Remove(absPath); err != nil {
				return err
			} else {
				le.Debugf("removed old output: %s", oldOutputPath)
			}
		}
	}
	return nil
}

// buildInputManifest creates an InputManifest from the build results.
func (c *Controller) buildInputManifest(
	sourcePath string,
	viteBuildResult *viteBuildResult,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	viteConfigPaths []string,
	webPkgSrcFiles []string,
) (*bldr_manifest_builder.InputManifest, error) {
	inputManifestMeta := &InputManifestMeta{
		WebPkgRefs:      viteBuildResult.webPkgRefs,
		WebPkgs:         webPkgs,
		ViteBundles:     viteBuildResult.viteBundles,
		ViteConfigPaths: viteConfigPaths,
		ViteOutputs:     viteBuildResult.viteOutputs,
	}
	inputManifestMetaBin, err := inputManifestMeta.MarshalVT()
	if err != nil {
		return nil, err
	}

	updatedManifestMeta := &bldr_manifest_builder.InputManifest{Metadata: inputManifestMetaBin}
	inputFileKinds := map[InputFileKind][]string{
		InputFileKind_InputFileKind_VITE: viteBuildResult.viteSrcFiles,
	}

	if len(webPkgSrcFiles) != 0 {
		inputFileKinds[InputFileKind_InputFileKind_WEB_PKG] = webPkgSrcFiles
	}

	for kind, srcPaths := range inputFileKinds {
		meta := &InputFileMeta{Kind: kind}
		metaBin, err := meta.MarshalVT()
		if err != nil {
			return nil, err
		}

		srcPathsCopy := make([]string, len(srcPaths))
		copy(srcPathsCopy, srcPaths)
		err = fsutil.ConvertPathsToRelative(sourcePath, srcPathsCopy)
		if err != nil {
			return nil, err
		}

		for _, srcPath := range srcPathsCopy {
			updatedManifestMeta.Files = append(updatedManifestMeta.Files, &bldr_manifest_builder.InputManifest_File{
				Path:     srcPath,
				Metadata: metaBin,
			})
		}
	}
	updatedManifestMeta.SortFiles()

	return updatedManifestMeta, nil
}

// tryFastRebuild attempts to perform a fast rebuild if conditions are met.
// Returns nil, nil if fast rebuild is not applicable or if web pkg refs changed.
func (c *Controller) tryFastRebuild(
	ctx context.Context,
	le *logrus.Entry,
	builderConf *bldr_manifest_builder.BuilderConfig,
	busEngine world.Engine,
	manifestID,
	sourcePath,
	distSourcePath,
	workingPath,
	outDistPath,
	outAssetsPath string,
	prevResult *bldr_manifest_builder.BuilderResult,
	changedFiles []*bldr_manifest_builder.InputManifest_File,
) (*bldr_manifest_builder.InputManifest, error) {
	// Check out the previous result to disk
	prevManifestRef := prevResult.GetManifestRef()
	_, err := builderConf.CheckoutManifest(
		ctx,
		le,
		busEngine.AccessWorldState,
		prevManifestRef.GetManifestRef(),
		outDistPath,
		outAssetsPath,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check out previous manifest")
	}

	prevInputManifest := prevResult.GetInputManifest()

	// Skip if there is no previous result or no changed files
	if len(changedFiles) == 0 || len(prevInputManifest.GetFiles()) == 0 {
		return nil, nil
	}

	// Skip if there is no valid input manifest metadata
	prevMetaBin := prevInputManifest.Metadata
	if len(prevMetaBin) == 0 {
		return nil, nil
	}
	inputMeta := &InputManifestMeta{}
	if err := inputMeta.UnmarshalVT(prevMetaBin); err != nil {
		return nil, errors.Wrap(err, "unmarshal input metadata")
	}

	// If any non-vite assets changed, skip fast rebuild
	meta := &InputFileMeta{}
	for _, changedFile := range changedFiles {
		meta.Reset()
		err := meta.UnmarshalVT(changedFile.GetMetadata())
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse file metadata")
		}
		kind := meta.GetKind()
		if kind != InputFileKind_InputFileKind_VITE {
			// Skip fast rebuild: non-vite asset changed
			return nil, nil
		}
	}

	le.Info("performing fast rebuild")

	// Process each bundle
	bundleList, err := BuildViteBundleMeta(inputMeta.GetViteBundles())
	if err != nil {
		return nil, err
	}

	// Build Vite bundles
	viteBuildResult, err := c.buildViteBundles(
		ctx,
		le,
		distSourcePath,
		sourcePath,
		workingPath,
		inputMeta.GetViteConfigPaths(),
		bundleList,
		inputMeta.GetWebPkgs(),
		outAssetsPath,
		manifestID,
		false, // Not release mode for fast rebuild
	)
	if err != nil {
		return nil, err
	}

	// Clean up old vite outputs that are no longer needed
	err = c.cleanupOldViteOutputs(le, outAssetsPath, inputMeta.GetViteOutputs(), viteBuildResult.viteOutputs)
	if err != nil {
		return nil, err
	}

	// Compare web pkg refs to see if they changed
	// If so, we must perform a full rebuild to pick up the new refs + rebuild the web pkgs
	if !(&InputManifestMeta{WebPkgRefs: inputMeta.WebPkgRefs}).EqualVT(&InputManifestMeta{WebPkgRefs: viteBuildResult.webPkgRefs}) {
		le.Info("references to web pkgs changed: forcing a full re-build")
		return nil, nil
	}

	// Build the updated input manifest, preserving non-vite files
	updatedInputManifest := prevInputManifest.CloneVT()
	updatedInputMeta := inputMeta.CloneVT()
	updatedInputMeta.ViteOutputs = viteBuildResult.viteOutputs

	// Remove all vite files from the set (we will add them back next)
	updatedInputManifest.Files = slices.DeleteFunc(updatedInputManifest.Files, func(f *bldr_manifest_builder.InputManifest_File) bool {
		meta.Reset()
		err := meta.UnmarshalVT(f.GetMetadata())
		if err != nil {
			return false
		}
		kind := meta.GetKind()
		return kind == InputFileKind_InputFileKind_VITE
	})

	// Add the updated vite files to the list
	viteSrcFilesCopy := make([]string, len(viteBuildResult.viteSrcFiles))
	copy(viteSrcFilesCopy, viteBuildResult.viteSrcFiles)
	if err := fsutil.ConvertPathsToRelative(sourcePath, viteSrcFilesCopy); err != nil {
		return nil, err
	}
	viteFileMeta := &InputFileMeta{Kind: InputFileKind_InputFileKind_VITE}
	viteFileMetaBin, err := viteFileMeta.MarshalVT()
	if err != nil {
		return nil, err
	}
	for _, srcPath := range viteSrcFilesCopy {
		updatedInputManifest.Files = append(updatedInputManifest.Files, &bldr_manifest_builder.InputManifest_File{
			Path:     srcPath,
			Metadata: viteFileMetaBin,
		})
	}
	updatedInputManifest.SortFiles()

	// Encode the updated metadata
	updMeta, err := updatedInputMeta.MarshalVT()
	if err != nil {
		return nil, err
	}
	updatedInputManifest.Metadata = updMeta

	le.Debug("fast rebuild complete")
	return updatedInputManifest, nil
}

// performFullRebuild performs a complete rebuild including web packages.
func (c *Controller) performFullRebuild(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
	builderConf *bldr_manifest_builder.BuilderConfig,
	busEngine world.Engine,
	manifestID,
	sourcePath,
	distSourcePath,
	workingPath,
	outDistPath,
	outAssetsPath string,
	buildType bldr_manifest.BuildType,
	isRelease bool,
) (*bldr_manifest_builder.InputManifest, error) {
	// Clean/create build directories
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		return nil, err
	}

	// Build base config
	buildCtrlConf := conf.CloneVT()
	if buildCtrlConf == nil {
		buildCtrlConf = &Config{}
	}

	// Apply the per-build-type configs
	buildCtrlConf.FlattenBuildTypes(buildType)

	// Apply the per-platform-type configs
	buildCtrlConf.FlattenPlatformTypes(bldr_platform.NewNonePlatform())

	// Call any pre-build hooks
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, busEngine)
		if err != nil {
			return nil, err
		}

		// Merge the returned config
		buildCtrlConf.Merge(res.GetConfig())
	}

	// Process each bundle
	bundleList, err := BuildViteBundleMeta(buildCtrlConf.GetBundles())
	if err != nil {
		return nil, err
	}

	// Build Vite bundles
	viteBuildResult, err := c.buildViteBundles(
		ctx,
		le,
		distSourcePath,
		sourcePath,
		workingPath,
		buildCtrlConf.GetViteConfigPaths(),
		bundleList,
		buildCtrlConf.GetWebPkgs(),
		outAssetsPath,
		manifestID,
		isRelease,
	)
	if err != nil {
		return nil, err
	}

	// Run esbuild on the web pkgs (if any)
	var webPkgSrcFiles []string
	if len(viteBuildResult.webPkgRefs) != 0 {
		outWebPkgsPath := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
		_, webPkgSrcFiles, err = web_pkg_esbuild.BuildWebPkgsEsbuild(
			ctx,
			le,
			sourcePath,
			viteBuildResult.webPkgRefs,
			outWebPkgsPath,
			bldr_plugin.PluginWebPkgHttpPrefix,
			isRelease,
			[]string{filepath.Join(sourcePath, "node_modules")},
		)
		if err != nil {
			return nil, err
		}
	}

	// Build the input manifest
	return c.buildInputManifest(
		sourcePath,
		viteBuildResult,
		buildCtrlConf.GetWebPkgs(),
		buildCtrlConf.GetViteConfigPaths(),
		webPkgSrcFiles,
	)
}

// GetSupportedPlatforms returns the base platform IDs this compiler supports.
// Returns nil because vite is a sub-manifest builder that produces platform-agnostic JS bundles.
func (c *Controller) GetSupportedPlatforms() []string {
	return nil
}

// _ is a type assertion
var _ bldr_manifest_builder.Controller = ((*Controller)(nil))
