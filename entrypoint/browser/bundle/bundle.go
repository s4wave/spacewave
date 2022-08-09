package entrypoint_browser_bundle

import (
	"io/ioutil"
	"os"
	"path"

	util_esbuild "github.com/aperturerobotics/bldr/util/esbuild"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

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
		LogLevel: esbuild.LogLevelDebug,

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
func BuildRendererBundle(le *logrus.Entry, repoRoot, buildDir string, minify bool) error {
	le.Debug("generating web renderer bundle")

	// index.html
	webSrcDir := path.Join(repoRoot, "web")
	indexHtmlPath := path.Join(webSrcDir, "index.html")
	ihtml, err := ioutil.ReadFile(indexHtmlPath)
	if err != nil {
		return err
	}
	rendererHtmlOut := path.Join(buildDir, "index.html")
	err = ioutil.WriteFile(rendererHtmlOut, ihtml, 0644)
	if err != nil {
		return err
	}

	// entrypoint
	webEntrypointOut := path.Join(buildDir, "entrypoint")
	rendererBuildOpts := BrowserEntrypointBuildOpts(repoRoot, minify)
	rendererBuildOpts.Outdir = webEntrypointOut
	rendererBuildOpts.Write = true
	if !minify {
		rendererBuildOpts.Sourcemap = esbuild.SourceMapLinked
	}
	res := esbuild.Build(rendererBuildOpts)
	return util_esbuild.BuildResultToErr(res)
}

// BuildRuntimeBundle copies all runtime files including runtime.wasm to the bundle.
func BuildRuntimeBundle(le *logrus.Entry, repoRoot, buildDir string, minify bool) error {
	// runtime
	runtimeOut := path.Join(buildDir, "runtime")
	if err := os.MkdirAll(runtimeOut, 0755); err != nil {
		return err
	}

	// runtime: web worker entrypoint: wasm
	runtimeEntrypointSrcDir := path.Join(repoRoot, "entrypoint", "browser")
	runtimeWasmPath := path.Join(runtimeEntrypointSrcDir, "runtime.wasm")
	if _, err := os.Stat(runtimeWasmPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("runtime.wasm: not found: please run build-runtime-wasm first")
		}
		return err
	}
	runtimeWasmJsPath := path.Join(runtimeEntrypointSrcDir, "runtime-wasm.js")
	if _, err := os.Stat(runtimeWasmJsPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("runtime-wasm.js: not found: please run build-runtime-wasm first")
		}
		return err
	}

	runtimeWasmOut := path.Join(runtimeOut, "runtime.wasm")
	runtimeWasmJsOut := path.Join(runtimeOut, "runtime-wasm.js")
	if err := CopyFile(runtimeWasmOut, runtimeWasmPath, 0755); err != nil {
		return err
	}
	if err := CopyFile(runtimeWasmJsOut, runtimeWasmJsPath, 0755); err != nil {
		return err
	}

	// runtime: web worker entrypoint: gopherjs
	runtimeJsPath := path.Join(runtimeEntrypointSrcDir, "runtime-js.js")
	runtimeGopherJsPath := path.Join(runtimeEntrypointSrcDir, "runtime-gopherjs.js")
	if _, err := os.Stat(runtimeGopherJsPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("runtime-gopherjs.js: not found: please run build-runtime-gopherjs first")
		}
		return err
	}
	if _, err := os.Stat(runtimeJsPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("runtime-js.js: not found: please run build-runtime-gopherjs first")
		}
		return err
	}
	runtimeJsOut := path.Join(runtimeOut, "runtime-js.js")
	if err := CopyFile(runtimeJsOut, runtimeJsPath, 0755); err != nil {
		return err
	}
	runtimeGopherJsOut := path.Join(runtimeOut, "runtime-gopherjs.js")
	if err := CopyFile(runtimeGopherJsOut, runtimeGopherJsPath, 0755); err != nil {
		return err
	}

	return nil
}

// BuildBrowserBundle builds and outputs the web & service worker files.
func BuildBrowserBundle(le *logrus.Entry, repoRoot, buildDir string, minify bool) error {
	err := os.MkdirAll(buildDir, 0755)
	if err != nil {
		return err
	}

	// service worker
	if err := BuildServiceWorkerBundle(le, repoRoot, buildDir, minify); err != nil {
		return err
	}

	// renderer bundle
	if err := BuildRendererBundle(le, repoRoot, buildDir, minify); err != nil {
		return err
	}

	// runtime bundle
	if err := BuildRuntimeBundle(le, repoRoot, buildDir, minify); err != nil {
		return err
	}

	return nil
}
