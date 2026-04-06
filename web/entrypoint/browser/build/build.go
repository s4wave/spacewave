//go:build !js

package browser_build

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
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
	buildType bldr_manifest.BuildType,
	useTinygo bool,
	runtimeWasmPath string,
) error {
	le.Info("building runtime-wasm.mjs")

	wasmExecFile, err := gocompiler.GetWasmExecPath(ctx, le, useTinygo)
	if err != nil {
		return err
	}

	// Build runtime wasm entrypoint
	entrypointJsDir := filepath.Join(bldrDistRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-wasm.mjs")

	minify := buildType.IsRelease()
	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointJsDir, minify)
	opts.EntryPoints = []string{"runtime-wasm.ts"}
	opts.Outfile = runtimeJsOut
	opts.Write = true

	if useTinygo {
		nodeStubsLoc := filepath.Join(bldrDistRoot, nodeStubsPath)
		nodeStubsLoc, err = filepath.Rel(entrypointJsDir, nodeStubsLoc)
		if err != nil {
			return err
		}
		opts.Inject = append(opts.Inject, nodeStubsLoc)
		opts.External = append(opts.External, "fs", "crypto", "util")
	}
	opts.Inject = append(opts.Inject, wasmExecFile)

	if runtimeWasmPath != "" {
		opts.Define["BLDR_RUNTIME_WASM"] = strconv.Quote(runtimeWasmPath)
	}

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}

// BuildSqliteWorkerEntrypoint builds the sqlite dedicated worker entrypoint.
//
// builds to buildDir/sqlite-worker.mjs
// assetPublicPath is the URL prefix for file-loader assets (e.g. "/entrypoint/").
func BuildSqliteWorkerEntrypoint(
	le *logrus.Entry,
	stateDir string,
	bldrDistRoot string,
	buildDir string,
	buildType bldr_manifest.BuildType,
	assetPublicPath string,
) error {
	le.Info("building sqlite-worker.mjs")

	minify := buildType.IsRelease()
	opts := entrypoint_browser_bundle.BrowserBuildOpts(bldrDistRoot, minify)
	opts.EntryPoints = []string{"web/runtime/wasm/sqlite/worker.ts"}
	opts.Outfile = filepath.Join(buildDir, "sqlite-worker.mjs")
	opts.PublicPath = assetPublicPath
	opts.Write = true

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}
	return copySqliteWorkerSidecars(stateDir, bldrDistRoot, buildDir)
}

// copySqliteWorkerSidecars copies sqlite worker sidecar files expected to live
// adjacent to sqlite-worker.mjs.
func copySqliteWorkerSidecars(stateDir, bldrDistRoot, buildDir string) error {
	buildPkgsDir, _ := filepath.Abs(filepath.Join(stateDir, "build-web-pkgs"))
	srcCandidates := []string{
		filepath.Join(
			buildPkgsDir,
			"node_modules",
			"@aptre",
			"sqlite-wasm",
			"dist",
			"sqlite3-opfs-async-proxy.js",
		),
		filepath.Join(
			bldrDistRoot,
			"node_modules",
			"@aptre",
			"sqlite-wasm",
			"dist",
			"sqlite3-opfs-async-proxy.js",
		),
		filepath.Join(
			bldrDistRoot,
			"dist",
			"deps",
			"node_modules",
			"@aptre",
			"sqlite-wasm",
			"dist",
			"sqlite3-opfs-async-proxy.js",
		),
	}

	var srcPath string
	for _, candidate := range srcCandidates {
		if _, err := os.Stat(candidate); err == nil {
			srcPath = candidate
			break
		}
	}
	if srcPath == "" {
		return os.ErrNotExist
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(buildDir, "sqlite3-opfs-async-proxy.js"), data, 0o644)
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
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}
