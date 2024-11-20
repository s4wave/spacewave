package entrypoint_browser_bundle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_platform_npm "github.com/aperturerobotics/bldr/platform/npm"
	"github.com/aperturerobotics/bldr/util/npm"
	web_entrypoint_index "github.com/aperturerobotics/bldr/web/entrypoint/index"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/esbuild/build"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	"github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EsbuildLogLevel is the log level when bundling the bundle.
var EsbuildLogLevel = esbuild.LogLevelWarning

// DefaultBanner is the default banner applied to code files.
func DefaultBanner() map[string]string {
	return map[string]string{
		"js": "// © 2018-2024 Aperture Robotics, LLC. <support@aperture.us>\n// All rights reserved.",
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

		Target:      esbuild.ES2022,
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
	buildOpts.External = slices.Clone(web_pkg_esbuild.BldrExternal)
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

// BuildRendererBundle builds the web renderer bundle files.
//
// runtimeStartupPath is the path to the startup js module to load for the react app entrypoint (can be empty).
// entrypointHash, if set, builds into /entrypoint/{entrypointHash}/...
func BuildRendererBundle(
	le *logrus.Entry,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeStartupPath,
	entrypointHash string,
	minify bool,
) error {
	le.Debug("generating web renderer bundle")

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
	importMap := web_entrypoint_index.ImportMap{
		Imports: map[string]string{
			"react":                   pkgsPathPrefix + "react/index.mjs",
			"react/jsx-runtime":       pkgsPathPrefix + "react/jsx-runtime.mjs",
			"react-dom":               pkgsPathPrefix + "react-dom/index.mjs",
			"react-dom/client":        pkgsPathPrefix + "react-dom/client.mjs",
			"react-dom/test-utils":    pkgsPathPrefix + "react-dom/test-utils.mjs",
			"@aptre/bldr":             pkgsPathPrefix + "@aptre/bldr/index.mjs",
			"@aptre/bldr-react":       pkgsPathPrefix + "@aptre/bldr-react/index.mjs",
			"@aptre/protobuf-es-lite": pkgsPathPrefix + "@aptre/protobuf-es-lite/index.mjs",
		},
	}

	// render index.html
	indexHtml, err := web_entrypoint_index.RenderIndexHTML(web_entrypoint_index.IndexData{
		ImportMap:      importMap,
		EntrypointPath: entrypointImportPath,
	})
	if err != nil {
		return err
	}
	rendererHtmlOut := filepath.Join(buildDir, "index.html")
	err = os.WriteFile(rendererHtmlOut, []byte(indexHtml), 0o644)
	if err != nil {
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

	if runtimeStartupPath != "" {
		rendererBuildOpts.Define["BLDR_STARTUP_JS"] = strconv.Quote(runtimeStartupPath)
	}

	if !minify {
		rendererBuildOpts.Sourcemap = esbuild.SourceMapLinked
	}

	res := esbuild.Build(rendererBuildOpts)
	return bldr_esbuild_build.BuildResultToErr(res)
}

// BuildBrowserBundle builds and outputs the web & service worker files.
//
// runtimeStartupPath is the path to the startup js module to load for the react app entrypoint (can be empty).
// entrypointHash, if set, builds into /entrypoint/{entrypointHash}/...
func BuildBrowserBundle(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeStartupPath string,
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

	// replace the filename in runtimeSwPath with the sw filename
	runtimeSwPath = filepath.Join(filepath.Dir(runtimeSwPath), swFilename)

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
	if err := BuildWebPkgsBundle(ctx, le, bldrNativePlatform, bldrDistRoot, entrypointDir, pkgsPathPrefix, minify, devMode); err != nil {
		return err
	}

	// renderer bundle
	if err := BuildRendererBundle(le, bldrDistRoot, buildDir, runtimeJsPath, runtimeSwPath, runtimeStartupPath, entrypointHash, minify); err != nil {
		return err
	}

	return nil
}

// BuildWebPkgsBundle builds the web pkg bundle files.
// pathPrefix is the prefix to prepend to /pkgs/ for pkg paths
func BuildWebPkgsBundle(ctx context.Context, le *logrus.Entry, plat bldr_platform.Platform, bldrDistRoot, buildDir, pathPrefix string, minify, devMode bool) error {
	// build to pkgs/
	outDir := filepath.Join(buildDir, "pkgs")

	// make temporary dir to build web pkgs
	buildPkgsDir := filepath.Join(buildDir, "build-web-pkgs")
	if err := fsutil.CleanCreateDir(buildPkgsDir); err != nil {
		return err
	}

	// copy package.json into it
	if err := fsutil.CopyFile(
		filepath.Join(buildPkgsDir, "package.json"),
		filepath.Join(bldrDistRoot, "dist/deps/package.json"),
		0o644,
	); err != nil {
		return err
	}

	// npm install
	npmPlat, err := bldr_platform_npm.PlatformToNpm(plat)
	if err != nil {
		return err
	}

	le.
		WithField("npm-platform", npmPlat.Platform).
		WithField("npm-arch", npmPlat.Arch).
		WithField("npm-pkg", []string{"react", "react-dom"}).
		Debug("downloading dist deps with npm")
	archFlags := npmPlat.ToNpmFlags()
	args := []string{"install"}
	args = append(args, npm.NpmFlags...)
	args = append(args, "--prefix", buildPkgsDir)
	args = append(args, archFlags...)
	cmd := exec.NewCmd("npm", args...)
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	// web pkgs we distribute with bldr
	refs := web_pkg_esbuild.GetBldrDistWebPkgRefs(buildPkgsDir, bldrDistRoot)

	// if we are in development mode: include test-utils to react-dom
	if devMode {
		refs[1].Imports = append(refs[1].Imports, "test-utils.js")
	}

	_, _, err = web_pkg_esbuild.BuildWebPkgsEsbuild(
		ctx,
		le,
		buildDir,
		refs,
		outDir,
		// pkgsPathPrefix+"",
		pathPrefix+"/pkgs/",
		minify,
	)
	if err != nil {
		return err
	}

	if err := fsutil.CleanDir(buildPkgsDir); err != nil {
		return err
	}

	return nil
}
