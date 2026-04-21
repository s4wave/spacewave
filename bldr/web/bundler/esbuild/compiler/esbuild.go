//go:build !js

package bldr_web_bundler_esbuild_compiler

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bldr_web_bundler_esbuild "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_esbuild "github.com/s4wave/spacewave/bldr/web/pkg/esbuild"
	"github.com/sirupsen/logrus"
)

// BuildEsbuildBundle builds an esbuild bundle with the given bundle args.
// Parameters:
// - le: logger entry
// - codeRootPath: root path of the source code
// - bundleID: identifier for the bundle
// - entrypoints: list of entrypoints to build
// - baseEsbuildOpts: base esbuild options to use
// - webPkgs: list of web packages to externalize
// - outAssetsPath: output path for assets
// - publicPath: public URL path prefix for assets
// - inlineSourcemaps: whether to inline sourcemaps
// - isRelease: whether this is a release build
// Returns:
// - Web package references used by the bundle
// - Metadata about the esbuild outputs
// - List of source files used by esbuild
// - Any error that occurred
func BuildEsbuildBundle(
	le *logrus.Entry,
	codeRootPath string,
	bundleID string,
	entrypoints []*bldr_web_bundler_esbuild.EsbuildBundleEntrypoint,
	baseEsbuildOpts *esbuild_api.BuildOptions,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	outAssetsPath string,
	publicPath string,
	inlineSourcemaps bool,
	isRelease bool,
) ([]*web_pkg.WebPkgRef, []*bldr_web_bundler_esbuild.EsbuildOutputMeta, []string, error) {
	var sourceFilesList []string
	var webPkgRefs []*web_pkg.WebPkgRef
	addWebPkgRef := func(webPkgID, webPkgRoot, webPkgSubPath string) {
		webPkgRefs, _ = web_pkg.
			WebPkgRefSlice(webPkgRefs).
			AppendWebPkgRef(webPkgID, webPkgRoot, webPkgSubPath)
	}

	// construct build options
	buildOpts := web_pkg_esbuild.BuildEsbuildBuildOpts(
		le,
		codeRootPath,
		outAssetsPath,
		publicPath,
		isRelease,
		true,
	)
	if inlineSourcemaps && !isRelease {
		buildOpts.Sourcemap = esbuild_api.SourceMapInlineAndExternal
	}

	// merge options set by baseEsbuildOpts
	if baseEsbuildOpts != nil {
		web_pkg_esbuild.MergeEsbuildBuildOpts(buildOpts, baseEsbuildOpts)
	}

	// Process each entrypoint
	entrypointInputPathsMap := make(map[string]*bldr_web_bundler_esbuild.EsbuildBundleEntrypoint)
	for _, entrypoint := range entrypoints {
		// Process input path
		inputPath := entrypoint.GetInputPath()
		if inputPath == "" {
			return nil, nil, nil, errors.New("input_path is required")
		}

		// Handle absolute paths
		if filepath.IsAbs(inputPath) {
			var err error
			inputPath = filepath.Join(codeRootPath, inputPath)
			inputPath, err = filepath.Rel(codeRootPath, inputPath)
			if err != nil {
				return nil, nil, nil, err
			}
		}

		// Ensure path is within code root
		inputPath = filepath.Join(codeRootPath, inputPath)
		inputPath, err := filepath.Rel(codeRootPath, inputPath)
		if err != nil {
			return nil, nil, nil, err
		}
		if strings.HasPrefix(inputPath, "../") {
			return nil, nil, nil, errors.Errorf("entrypoint cannot be outside code root: %s", inputPath)
		}

		// Add to esbuild entrypoints
		entrypointInputPathsMap[inputPath] = entrypoint
		buildOpts.EntryPointsAdvanced = append(buildOpts.EntryPointsAdvanced, esbuild_api.EntryPoint{
			InputPath:  inputPath,
			OutputPath: entrypoint.GetOutputPath(),
		})
	}

	// add the bldr plugin
	buildOpts.Plugins = append(
		buildOpts.Plugins,
		web_pkg_esbuild.BuildEsbuildPlugin(
			le,
			bldr_web_bundler.WebPkgRefConfigSlice(webPkgs).ToIdList(),
			addWebPkgRef,
		),
	)

	// https://github.com/evanw/esbuild/issues/1921
	// NOTE: we can't use async import() here since require() is called w/o await.
	// This fixes an issue with esbuild where dynamic imports don't work correctly in certain environments
	web_pkg_esbuild.FixEsbuildIssue1921(buildOpts)

	// compile the bundle
	le.Debugf("compiling bundle with esbuild: %s", bundleID)
	result := esbuild_api.Build(*buildOpts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return nil, nil, nil, err
	}
	if len(result.OutputFiles) == 0 {
		return nil, nil, nil, errors.New("esbuild: expected at least one output file but got none")
	}

	metaFile, err := bldr_esbuild_build.ParseEsbuildMetafile([]byte(result.Metafile))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "parse esbuild metafile")
	}

	// Get the list of source files to watch
	for inFilePath := range metaFile.Inputs {
		sourceFilesList = append(sourceFilesList, inFilePath)
	}

	// Build output metadata
	esbuildOutputMeta := bldr_web_bundler_esbuild.BuildEsbuildOutputMetas(metaFile, entrypoints)
	for _, meta := range esbuildOutputMeta {
		// Transform paths to be relative to assets dir
		metaPath := filepath.Join(codeRootPath, meta.Path)
		metaPath, err := filepath.Rel(outAssetsPath, metaPath)
		if err != nil {
			return nil, nil, nil, err
		}
		meta.Path = metaPath

		if meta.GetCssBundlePath() != "" {
			metaCssPath := filepath.Join(codeRootPath, meta.CssBundlePath)
			metaCssPath, err := filepath.Rel(outAssetsPath, metaCssPath)
			if err != nil {
				return nil, nil, nil, err
			}
			meta.CssBundlePath = metaCssPath
		}
	}

	// Sort and return
	esbuildOutputMeta = bldr_web_bundler_esbuild.SortEsbuildOutputMetas(esbuildOutputMeta)
	return webPkgRefs, esbuildOutputMeta, sourceFilesList, nil
}

// BuildEsbuildBundleMeta builds the bundle metadata from the bundles.
//
// Deduplicates and combines together multiple entrypoints for the same bundle.
func BuildEsbuildBundleMeta(bundles []*EsbuildBundleMeta) ([]*EsbuildBundleMeta, error) {
	// bundleMap is the map of bundle-id to bundle-def
	bundleMap := make(map[string]*EsbuildBundleMeta)
	for _, bundle := range bundles {
		bundleID := bundle.GetId()
		if bundleID == "" {
			bundleID = "default"
		}

		existingBundle, exists := bundleMap[bundleID]
		if exists {
			// Merge the bundles by appending entrypoints
			existingBundle.Entrypoints = append(existingBundle.Entrypoints, bundle.GetEntrypoints()...)
		} else {
			bundleMap[bundleID] = bundle
		}
	}

	out := slices.Collect(maps.Values(bundleMap))
	slices.SortFunc(out, func(a, b *EsbuildBundleMeta) int {
		return strings.Compare(a.GetId(), b.GetId())
	})
	return out, nil
}
