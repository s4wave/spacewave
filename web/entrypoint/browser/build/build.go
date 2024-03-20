package browser_build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// webEntrypointBrowserDir is the repo sub-dir for the browser entrypoint.
const webEntrypointBrowserDir = "web/entrypoint/browser"

// BuildWasmRuntimeEntrypoint builds the wasm runtime entrypoint.
//
// builds to buildDir/runtime-wasm.mjs
func BuildWasmRuntimeEntrypoint(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot string,
	buildDir string,
	buildType bldr_manifest.BuildType,
	buildPlatform bldr_platform.Platform,
) error {
	le.Info("building runtime-wasm.mjs")
	goRootDir := runtime.GOROOT()
	wasmExecFile := filepath.Join(goRootDir, "misc/wasm/wasm_exec.js")
	if _, err := os.Stat(wasmExecFile); err != nil {
		return errors.Wrapf(err, "cannot find wasm_exec.js in goroot: %s", wasmExecFile)
	}

	// Build runtime wasm entrypoint
	entrypointJsDir := filepath.Join(bldrDistRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-wasm.mjs")

	minify := buildType.IsRelease()
	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointJsDir, minify)
	opts.EntryPoints = []string{"runtime-wasm.ts"}
	opts.Inject = append(opts.Inject, wasmExecFile)
	opts.Outfile = runtimeJsOut
	opts.Write = true

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}

// BuildWsRuntime builds the WebSocket dev runtime entrypoint.
//
// builds to buildDir/runtime-ws.mjs
func BuildWsRuntime(ctx context.Context, le *logrus.Entry, bldrDistRoot, buildDir string, minify bool) error {
	le.Info("building runtime-ws.mjs")
	entrypointJsDir := filepath.Join(bldrDistRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-ws.mjs")

	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointJsDir, minify)
	opts.EntryPoints = []string{"runtime-ws.ts"}
	opts.Outfile = runtimeJsOut
	opts.Write = true

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}
