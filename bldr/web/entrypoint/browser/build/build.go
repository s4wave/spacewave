//go:build !js

package browser_build

import (
	"context"
	"path/filepath"
	"strconv"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

// webEntrypointBrowserDir is the repo sub-dir for the browser entrypoint.
const webEntrypointBrowserDir = "web/entrypoint/browser"

// nodeStubsPath is the repo sub-dir for the node stubs
const nodeStubsPath = "web/runtime/wasm/node-stubs.js"

// BuildWasmRuntimeEntrypoint builds the wasm runtime entrypoint.
//
// runtimeWasmPath should be the relative path to runtime.wasm from runtime-wasm.js
// this defaults to "./runtime.wasm"
//
// builds to buildDir/runtime-wasm.mjs
func BuildWasmRuntimeEntrypoint(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot string,
	buildDir string,
	minify bool,
	sourcemaps bool,
	useTinygo bool,
	runtimeWasmPath string,
) error {
	le.Info("building runtime-wasm.mjs")

	// Resolve the wasm execution shim for the selected compiler.
	wasmExecFile, err := gocompiler.GetWasmExecPath(ctx, le, useTinygo)
	if err != nil {
		return err
	}

	// Configure the browser runtime entrypoint and output path.
	entrypointJsDir := filepath.Join(bldrDistRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-wasm.mjs")

	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointJsDir, minify, sourcemaps)
	opts.EntryPoints = []string{"runtime-wasm.ts"}
	opts.Outfile = runtimeJsOut
	opts.Write = true

	// Apply TinyGo fallbacks when requested.
	if useTinygo {
		nodeStubsLoc := filepath.Join(bldrDistRoot, nodeStubsPath)
		nodeStubsLoc, err = filepath.Rel(entrypointJsDir, nodeStubsLoc)
		if err != nil {
			return err
		}
		opts.Inject = append(opts.Inject, nodeStubsLoc)
		entrypoint_browser_bundle.ApplyTinyGoNodeFallbacks(&opts)
		entrypoint_browser_bundle.ApplyTinyGoWasmExecPatches(&opts, wasmExecFile)
	}

	// Add the wasm execution shim and optional runtime path define.
	opts.Inject = append(opts.Inject, wasmExecFile)

	if runtimeWasmPath != "" {
		opts.Define["BLDR_RUNTIME_WASM"] = strconv.Quote(runtimeWasmPath)
	}

	// Build the configured browser runtime bundle.
	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}
	return nil
}

// BuildWsRuntime builds the WebSocket dev runtime entrypoint.
//
// builds to buildDir/runtime-ws.mjs
func BuildWsRuntime(ctx context.Context, le *logrus.Entry, bldrDistRoot, buildDir string, minify, sourcemaps bool) error {
	le.Info("building runtime-ws.mjs")

	// Configure the WebSocket runtime entrypoint and output path.
	entrypointJsDir := filepath.Join(bldrDistRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-ws.mjs")

	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointJsDir, minify, sourcemaps)
	opts.EntryPoints = []string{"runtime-ws.ts"}
	opts.Outfile = runtimeJsOut
	opts.Write = true

	// Build the configured WebSocket runtime bundle.
	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}
	return nil
}
