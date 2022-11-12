package entrypoint_browser_bundle

import (
	"os"
	"path"
	"strconv"

	util_esbuild "github.com/aperturerobotics/bldr/util/esbuild"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// EsbuildLogLevel is the log level when bundling the bundle.
var EsbuildLogLevel = esbuild.LogLevelInfo

func DefaultBanner() map[string]string {
	return map[string]string{
		"js": "// github.com/aperturerobotics/bldr/entrypoint/browser/bundle",
	}
}

// BrowserBuildOpts are general options for building for the browser.
func BrowserBuildOpts(repoRoot string, minify bool) esbuild.BuildOptions {
	return esbuild.BuildOptions{
		Bundle:   true,
		Target:   esbuild.ES2020,
		Format:   esbuild.FormatDefault,
		Platform: esbuild.PlatformBrowser,
		LogLevel: EsbuildLogLevel,

		AbsWorkingDir: repoRoot,
		Banner:        DefaultBanner(),
		Define: map[string]string{
			"BLDR_IS_BROWSER": "true",
		},

		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
	}
}

// BrowserEntrypointBuildOpts creates the BuildOpts for the root browser entrypoint
func BrowserEntrypointBuildOpts(repoRoot string, minify bool) esbuild.BuildOptions {
	buildOpts := BrowserBuildOpts(repoRoot, minify)
	buildOpts.EntryPointsAdvanced = []esbuild.EntryPoint{{
		InputPath:  "web/entrypoint/entrypoint.tsx",
		OutputPath: "entrypoint",
	}}
	return buildOpts
}

// ServiceWorkerBuildOpts creates the BuildOpts for the service worker
func ServiceWorkerBuildOpts(repoRoot string, minify bool) esbuild.BuildOptions {
	baseConfig := BrowserEntrypointBuildOpts(repoRoot, minify)
	baseConfig.EntryPoints = []string{"web/bldr/service-worker.ts"}
	baseConfig.EntryPointsAdvanced = nil
	return baseConfig
}

// BuildServiceWorkerBundle builds specifically the service worker files.
func BuildServiceWorkerBundle(le *logrus.Entry, repoRoot, buildDir string, minify bool) error {
	le.Debug("generating service-worker bundle")
	swOut := path.Join(buildDir, "sw.js")
	swOpts := ServiceWorkerBuildOpts(repoRoot, minify)
	swOpts.Outfile = swOut
	swOpts.Write = true
	if !minify {
		swOpts.Sourcemap = esbuild.SourceMapInline
	}
	return util_esbuild.BuildResultToErr(esbuild.Build(swOpts))
}

// BuildRendererBundle builds the web renderer bundle files.
func BuildRendererBundle(le *logrus.Entry, repoRoot, buildDir, runtimeJsPath string, minify bool) error {
	le.Debug("generating web renderer bundle")

	// index.html
	webSrcDir := path.Join(repoRoot, "web")
	indexHtmlPath := path.Join(webSrcDir, "index.html")
	ihtml, err := os.ReadFile(indexHtmlPath)
	if err != nil {
		return err
	}
	rendererHtmlOut := path.Join(buildDir, "index.html")
	err = os.WriteFile(rendererHtmlOut, ihtml, 0644)
	if err != nil {
		return err
	}

	// entrypoint
	webEntrypointOut := path.Join(buildDir, "entrypoint")
	rendererBuildOpts := BrowserEntrypointBuildOpts(repoRoot, minify)
	rendererBuildOpts.Outdir = webEntrypointOut
	rendererBuildOpts.Write = true
	if runtimeJsPath != "" {
		rendererBuildOpts.Define["BLDR_RUNTIME_JS"] = strconv.Quote(runtimeJsPath)
	}
	if !minify {
		rendererBuildOpts.Sourcemap = esbuild.SourceMapLinked
	}
	res := esbuild.Build(rendererBuildOpts)
	return util_esbuild.BuildResultToErr(res)
}

// BuildBrowserBundle builds and outputs the web & service worker files.
//
// NOTE: we expect runtime-wasm.js to exist at buildDir/runtime/runtime-wasm.js
func BuildBrowserBundle(le *logrus.Entry, repoRoot, buildDir, runtimeJsPath string, minify bool) error {
	err := os.MkdirAll(buildDir, 0755)
	if err != nil {
		return err
	}

	// service worker
	if err := BuildServiceWorkerBundle(le, repoRoot, buildDir, minify); err != nil {
		return err
	}

	// renderer bundle
	if err := BuildRendererBundle(le, repoRoot, buildDir, runtimeJsPath, minify); err != nil {
		return err
	}

	return nil
}
