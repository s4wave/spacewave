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
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/esbuild/build"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	"github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// EsbuildLogLevel is the log level when bundling the bundle.
var EsbuildLogLevel = esbuild.LogLevelWarning

func DefaultBanner() map[string]string {
	return map[string]string{
		"js": "// github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle",
	}
}

// BrowserBuildOpts are general options for building for the browser.
func BrowserBuildOpts(workingDir string, minify bool) esbuild.BuildOptions {
	sourceMap := esbuild.SourceMapNone
	if !minify {
		sourceMap = esbuild.SourceMapLinked
	}

	return esbuild.BuildOptions{
		AbsWorkingDir: workingDir,

		Target:      esbuild.ES2022,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformBrowser,
		LogLevel:    EsbuildLogLevel,
		TreeShaking: esbuild.TreeShakingTrue,
		Sourcemap:   sourceMap,

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
func ServiceWorkerBuildOpts(bldrDistRoot string, minify bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify)
	baseConfig.EntryPoints = []string{"web/bldr/service-worker.ts"}
	baseConfig.EntryPointsAdvanced = nil
	return baseConfig
}

// BuildServiceWorkerBundle builds specifically the service worker files.
func BuildServiceWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) error {
	le.Debug("generating service-worker bundle")
	swOut := filepath.Join(buildDir, "sw.mjs")
	swOpts := ServiceWorkerBuildOpts(bldrDistRoot, minify)
	swOpts.Outfile = swOut
	swOpts.Write = true
	if !minify {
		swOpts.Sourcemap = esbuild.SourceMapInline
	}
	swOpts.Define["BLDR_DEBUG"] = strconv.FormatBool(devMode)
	return bldr_esbuild_build.BuildResultToErr(esbuild.Build(swOpts))
}

// BuildRendererBundle builds the web renderer bundle files.
func BuildRendererBundle(le *logrus.Entry, bldrDistRoot, buildDir, runtimeJsPath string, minify bool) error {
	le.Debug("generating web renderer bundle")

	// index.html
	webSrcDir := filepath.Join(bldrDistRoot, "web")
	indexHtmlPath := filepath.Join(webSrcDir, "index.html")
	ihtml, err := os.ReadFile(indexHtmlPath)
	if err != nil {
		return err
	}
	rendererHtmlOut := filepath.Join(buildDir, "index.html")
	err = os.WriteFile(rendererHtmlOut, ihtml, 0o644)
	if err != nil {
		return err
	}

	// entrypoint
	webEntrypointOut := filepath.Join(buildDir, "entrypoint")
	rendererBuildOpts := BrowserEntrypointBuildOpts(bldrDistRoot, minify)
	rendererBuildOpts.Outdir = webEntrypointOut
	rendererBuildOpts.Write = true

	if runtimeJsPath != "" {
		rendererBuildOpts.Define["BLDR_RUNTIME_JS"] = strconv.Quote(runtimeJsPath)
	}
	if !minify {
		rendererBuildOpts.Sourcemap = esbuild.SourceMapLinked
	}

	res := esbuild.Build(rendererBuildOpts)
	return bldr_esbuild_build.BuildResultToErr(res)
}

// BuildBrowserBundle builds and outputs the web & service worker files.
func BuildBrowserBundle(ctx context.Context, le *logrus.Entry, bldrDistRoot, buildDir, runtimeJsPath string, minify, devMode bool) error {
	err := os.MkdirAll(buildDir, 0o755)
	if err != nil {
		return err
	}

	// service worker
	if err := BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode); err != nil {
		return err
	}

	// web pkgs
	// use platform for linux -> node.js (react and react-dom don't care.)
	bldrNativePlatform, err := bldr_platform.ParseNativePlatform("native/linux/amd64")
	if err != nil {
		return err
	}
	if err := BuildWebPkgsBundle(ctx, le, bldrNativePlatform, bldrDistRoot, buildDir, minify, devMode); err != nil {
		return err
	}

	// renderer bundle
	if err := BuildRendererBundle(le, bldrDistRoot, buildDir, runtimeJsPath, minify); err != nil {
		return err
	}

	return nil
}

// BuildWebPkgsBundle builds the web pkg bundle files.
func BuildWebPkgsBundle(ctx context.Context, le *logrus.Entry, plat bldr_platform.Platform, bldrDistRoot, buildDir string, minify, devMode bool) error {
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
		// "./pkgs/",
		"/pkgs/",
		false,
	)
	if err != nil {
		return err
	}

	if err := fsutil.CleanDir(buildPkgsDir); err != nil {
		return err
	}

	return nil
}
