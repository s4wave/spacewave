package entrypoint_browser_bundle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/aperturerobotics/bldr/util/npm"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	web_entrypoint_index "github.com/aperturerobotics/bldr/web/entrypoint/index"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"
	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EsbuildLogLevel is the log level when bundling the bundle.
var EsbuildLogLevel = esbuild.LogLevelWarning

// DefaultBanner is the default banner applied to code files.
func DefaultBanner() map[string]string {
	return map[string]string{
		"js": "// © 2018-2025 Aperture Robotics, LLC. <support@aperture.us>\n// All rights reserved.",
	}
}

// BrowserBuildOpts are general options for building for the browser.
func BrowserBuildOpts(workingDir string, minify bool) esbuild.BuildOptions {
	sourceMap := esbuild.SourceMapNone
	if !minify {
		sourceMap = esbuild.SourceMapLinked
	}

	var drop esbuild.Drop
	if minify {
		drop = esbuild.DropDebugger
	}

	return esbuild.BuildOptions{
		AbsWorkingDir: workingDir,

		Target:      esbuild.ES2024,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformBrowser,
		LogLevel:    EsbuildLogLevel,
		TreeShaking: esbuild.TreeShakingTrue,
		Sourcemap:   sourceMap,
		Drop:        drop,

		Metafile:  false,
		Splitting: false,

		Banner: DefaultBanner(),
		Define: map[string]string{
			"BLDR_IS_BROWSER": "true",
		},

		Loader: map[string]esbuild.Loader{
			".woff":  esbuild.LoaderFile,
			".woff2": esbuild.LoaderFile,
			".png":   esbuild.LoaderFile,
			".jpg":   esbuild.LoaderFile,
			".jpeg":  esbuild.LoaderFile,
			".svg":   esbuild.LoaderFile,
			".gif":   esbuild.LoaderFile,
		},
		OutExtension: map[string]string{
			".js": ".mjs",
		},

		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,

		Bundle: true,
	}
}

// BrowserEntrypointBuildOpts creates the BuildOpts for the root browser entrypoint
func BrowserEntrypointBuildOpts(bldrDistRoot string, minify bool) esbuild.BuildOptions {
	buildOpts := BrowserBuildOpts(bldrDistRoot, minify)
	buildOpts.External = slices.Clone(web_pkg_external.BldrExternal)
	buildOpts.External = append(buildOpts.External, "tailwindcss")
	buildOpts.EntryPointsAdvanced = []esbuild.EntryPoint{{
		InputPath:  "web/entrypoint/entrypoint.tsx",
		OutputPath: "entrypoint",
	}}
	return buildOpts
}

// ServiceWorkerBuildOpts creates the BuildOpts for the service worker
func ServiceWorkerBuildOpts(bldrDistRoot string, minify, hash bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify)
	if hash {
		baseConfig.EntryNames = "sw-[hash]"
	} else {
		baseConfig.EntryNames = "sw"
	}
	baseConfig.EntryPoints = []string{"web/bldr/service-worker.ts"}
	baseConfig.EntryPointsAdvanced = nil
	return baseConfig
}

// SharedWorkerBuildOpts creates the BuildOpts for the shared worker
func SharedWorkerBuildOpts(bldrDistRoot string, minify, hash bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify)
	if hash {
		baseConfig.EntryNames = "shw-[hash]"
	} else {
		baseConfig.EntryNames = "shw"
	}
	baseConfig.EntryPoints = []string{"web/bldr/shared-worker.ts"}
	baseConfig.EntryPointsAdvanced = nil
	return baseConfig
}

// BuildServiceWorkerBundle builds specifically the service worker files.
//
// Returns the filename of the service worker output file (including the hash).
func BuildServiceWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) (string, error) {
	le.Debug("generating service-worker bundle")

	swOpts := ServiceWorkerBuildOpts(bldrDistRoot, minify, !devMode)
	swOpts.Outdir = buildDir
	swOpts.Write = true
	if !minify {
		swOpts.Sourcemap = esbuild.SourceMapInline
	}
	swOpts.Define["BLDR_DEBUG"] = strconv.FormatBool(devMode)
	result := esbuild.Build(swOpts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return "", err
	}
	if len(result.OutputFiles) != 1 {
		return "", errors.Errorf("expected %d output files but got %d", 1, len(result.OutputFiles))
	}
	return filepath.Base(result.OutputFiles[0].Path), nil
}

// BuildSharedWorkerBundle builds specifically the shared worker files.
//
// Returns the filename of the shared worker output file (including the hash).
func BuildSharedWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) (string, error) {
	le.Debug("generating shared-worker bundle")

	shwOpts := SharedWorkerBuildOpts(bldrDistRoot, minify, !devMode)
	shwOpts.Outdir = buildDir
	shwOpts.Write = true
	if !minify {
		shwOpts.Sourcemap = esbuild.SourceMapInline
	}
	shwOpts.Define["BLDR_DEBUG"] = strconv.FormatBool(devMode)
	result := esbuild.Build(shwOpts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return "", err
	}
	if len(result.OutputFiles) != 1 {
		return "", errors.Errorf("expected %d output files but got %d", 1, len(result.OutputFiles))
	}
	return filepath.Base(result.OutputFiles[0].Path), nil
}

// BuildRendererIndex builds the web renderer index.html.
func BuildRendererIndex(buildDir, entrypointHash string) error {
	// entrypoint import path
	entrypointImportPath := "./entrypoint"
	if entrypointHash != "" {
		entrypointImportPath += "/" + entrypointHash
	}
	entrypointImportPath += "/entrypoint.mjs"

	// pkgsPathPrefix is the path prefix to ./pkgs relative to index.html
	pkgsPathPrefix := "./entrypoint"
	if entrypointHash != "" {
		pkgsPathPrefix += "/" + entrypointHash
	}
	pkgsPathPrefix += "/pkgs/"

	// build the import map
	importMap := web_pkg_external.GetBldrDistImportMap(pkgsPathPrefix)

	// render index.html
	indexHtml, err := web_entrypoint_index.RenderIndexHTML(web_entrypoint_index.IndexData{
		ImportMap:      importMap,
		EntrypointPath: entrypointImportPath,
	})
	if err != nil {
		return err
	}
	rendererHtmlOut := filepath.Join(buildDir, "index.html")
	return os.WriteFile(rendererHtmlOut, []byte(indexHtml), 0o644)
}

// BuildRendererBundle builds the web renderer bundle files.
//
// webStartupSrcPath is the path to the startup js module to load for the react app entrypoint (can be empty).
// entrypointHash, if set, builds into /entrypoint/{entrypointHash}/...
func BuildRendererBundle(
	le *logrus.Entry,
	sourcesRoot,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeShwPath,
	webStartupSrcPath,
	entrypointHash string,
	minify bool,
) error {
	le.Debug("generating web renderer bundle")

	if err := BuildRendererIndex(buildDir, entrypointHash); err != nil {
		return err
	}

	// entrypoint
	webEntrypointOut := filepath.Join(buildDir, "entrypoint")
	if entrypointHash != "" {
		webEntrypointOut = filepath.Join(webEntrypointOut, entrypointHash)
	}

	rendererBuildOpts := BrowserEntrypointBuildOpts(bldrDistRoot, minify)
	rendererBuildOpts.Outdir = webEntrypointOut
	rendererBuildOpts.Write = true

	if runtimeJsPath != "" {
		rendererBuildOpts.Define["BLDR_RUNTIME_JS"] = strconv.Quote(runtimeJsPath)
	}

	if runtimeSwPath != "" {
		rendererBuildOpts.Define["BLDR_SW_JS"] = strconv.Quote(runtimeSwPath)
	}

	if runtimeShwPath != "" {
		rendererBuildOpts.Define["BLDR_SHW_JS"] = strconv.Quote(runtimeShwPath)
	}

	distSourcesDirToSourcesRoot, err := filepath.Rel(bldrDistRoot, sourcesRoot)
	if err != nil {
		return err
	}

	if webStartupSrcPath != "" {
		// esbuild interprets this path in an import() statement
		// we need a relative path from the entrypoint.tsx to the src path.
		// add an extra .. for the "web/entrypoint"
		webStartupSrcPathRel := filepath.Join(distSourcesDirToSourcesRoot, "../..", webStartupSrcPath)
		rendererBuildOpts.Define["BLDR_STARTUP_JS"] = strconv.Quote(webStartupSrcPathRel)
	}

	if !minify {
		rendererBuildOpts.Sourcemap = esbuild.SourceMapLinked
	}

	res := esbuild.Build(rendererBuildOpts)
	return bldr_esbuild_build.BuildResultToErr(res)
}

// BuildBrowserBundle builds and outputs the web & service worker files.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// webStartupSrcPath is the path to the startup js module to load for the react app entrypoint (can be empty).
// entrypointHash, if set, builds into /entrypoint/{entrypointHash}/...
func BuildBrowserBundle(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	sourcesRoot,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeShwPath,
	webStartupSrcPath string,
	entrypointHash string,
	minify,
	devMode bool,
) error {
	err := os.MkdirAll(buildDir, 0o755)
	if err != nil {
		return err
	}

	// service worker
	swFilename, err := BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
	if err != nil {
		return err
	}

	// shared worker
	shwFilename, err := BuildSharedWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
	if err != nil {
		return err
	}

	// replace the filename in runtimeSwPath with the sw filename
	runtimeSwPath = filepath.Join(filepath.Dir(runtimeSwPath), swFilename)
	// replace the filename in runtimeShwPath with the shw filename
	runtimeShwPath = filepath.Join(filepath.Dir(runtimeShwPath), shwFilename)

	// web pkgs
	// use platform for linux -> node.js (react and react-dom don't care.)
	bldrNativePlatform, err := bldr_platform.ParseNativePlatform("native/linux/amd64")
	if err != nil {
		return err
	}

	pkgsPathPrefix := "/entrypoint"
	if entrypointHash != "" {
		pkgsPathPrefix += "/" + entrypointHash
	}

	entrypointDir := filepath.Join(buildDir, "entrypoint")
	if entrypointHash != "" {
		entrypointDir = filepath.Join(entrypointDir, entrypointHash)
	}

	if err := BuildWebPkgsBundle(ctx, le, stateDir, bldrNativePlatform, bldrDistRoot, entrypointDir, pkgsPathPrefix, minify, devMode); err != nil {
		return err
	}

	// renderer bundle
	if err := BuildRendererBundle(le, sourcesRoot, bldrDistRoot, buildDir, runtimeJsPath, runtimeSwPath, runtimeShwPath, webStartupSrcPath, entrypointHash, minify); err != nil {
		return err
	}

	return nil
}

// BuildWebPkgsBundle builds the web pkg bundle files.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// pathPrefix is the prefix to prepend to /pkgs/ for pkg paths
func BuildWebPkgsBundle(ctx context.Context, le *logrus.Entry, stateDir string, plat bldr_platform.Platform, bldrDistRoot, buildDir, pathPrefix string, minify, devMode bool) error {
	// build to pkgs/
	outDir := filepath.Join(buildDir, "pkgs")

	// install dist deps (cached: skips if package.json unchanged)
	// Use stateDir (not buildDir) so the cache survives CleanCreateDir on the build output.
	buildPkgsDir, _ := filepath.Abs(filepath.Join(stateDir, "build-web-pkgs"))
	if err := npm.EnsureBunInstall(ctx, le, stateDir, filepath.Join(bldrDistRoot, "dist/deps/package.json"), buildPkgsDir); err != nil {
		return err
	}

	// web pkgs we distribute with bldr
	refs := web_pkg_external.GetBldrDistWebPkgRefs(buildPkgsDir, bldrDistRoot)

	// if we are in development mode: include test-utils to react-dom
	if devMode {
		for _, ref := range refs {
			if ref.WebPkgId == "react-dom" {
				ref.Imports = append(ref.Imports, "test-utils.js")
			}
		}
	}

	_, _, err := web_pkg_esbuild.BuildWebPkgsEsbuild(
		ctx,
		le,
		buildDir,
		refs,
		outDir,
		pathPrefix+"/pkgs/",
		minify,
		[]string{filepath.Join(buildPkgsDir, "node_modules")},
	)
	if err != nil {
		return err
	}

	return nil
}
