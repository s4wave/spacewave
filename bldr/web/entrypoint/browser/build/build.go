//go:build !js

package browser_build

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
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

	runtimeJsOut := filepath.Join(buildDir, "runtime-wasm.mjs")
	sourceMap := "none"
	if sourcemaps {
		sourceMap = "external"
	}
	inject := []string{wasmExecFile}
	var external []string
	var sourceOverrides map[string]string
	if useTinygo {
		nodeStubsLoc := bldr.ResolveDistSourcePath(bldrDistRoot, nodeStubsPath)
		inject = append([]string{nodeStubsLoc}, inject...)
		external = []string{"fs", "crypto", "util", "node:fs", "node:crypto", "node:util"}
		patched, err := entrypoint_browser_bundle.LoadTinyGoWasmExecSource(wasmExecFile)
		if err != nil {
			return err
		}
		sourceOverrides = map[string]string{wasmExecFile: patched}
	}
	defines := map[string]string{"BLDR_IS_BROWSER": "true"}
	if runtimeWasmPath != "" {
		defines["BLDR_RUNTIME_WASM"] = strconv.Quote(runtimeWasmPath)
	}
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		buildDir,
		bldrDistRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   buildDir,
			SourceRoot:   bldrDistRoot,
			OutputRoot:   buildDir,
			BldrDistRoot: bldrDistRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      "runtime-wasm",
				InputPath: bldr.ResolveDistSourcePath(bldrDistRoot, webEntrypointBrowserDir, "runtime-wasm.ts"),
			}},
			Format:          "es",
			Platform:        "browser",
			Target:          "es2024",
			EntryFileNames:  "runtime-wasm.mjs",
			ChunkFileNames:  "[name]-[hash].mjs",
			AssetFileNames:  "[name]-[hash][extname]",
			Sourcemap:       sourceMap,
			Minify:          minify,
			TreeShaking:     true,
			Banner:          entrypoint_browser_bundle.DefaultBanner()["js"],
			Defines:         defines,
			External:        external,
			Loaders:         map[string]string{".wasm": "asset"},
			Inject:          inject,
			SourceOverrides: sourceOverrides,
		},
	)
	if err != nil {
		return err
	}
	if result.GetEntrypointOutputs()["runtime-wasm"] != filepath.Base(runtimeJsOut) {
		return errors.Errorf("Wasm runtime output is %q", result.GetEntrypointOutputs()["runtime-wasm"])
	}
	return nil
}

// BuildWsRuntime builds the WebSocket dev runtime entrypoint.
//
// builds to buildDir/runtime-ws.mjs
func BuildWsRuntime(ctx context.Context, le *logrus.Entry, bldrDistRoot, buildDir string, minify, sourcemaps bool) error {
	le.Info("building runtime-ws.mjs")
	sourceMap := "none"
	if sourcemaps {
		sourceMap = "external"
	}
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		buildDir,
		bldrDistRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   buildDir,
			SourceRoot:   bldrDistRoot,
			OutputRoot:   buildDir,
			BldrDistRoot: bldrDistRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      "runtime-ws",
				InputPath: bldr.ResolveDistSourcePath(bldrDistRoot, webEntrypointBrowserDir, "runtime-ws.ts"),
			}},
			Format:         "es",
			Platform:       "browser",
			Target:         "es2024",
			EntryFileNames: "runtime-ws.mjs",
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      sourceMap,
			Minify:         minify,
			TreeShaking:    true,
			Banner:         entrypoint_browser_bundle.DefaultBanner()["js"],
			Defines:        map[string]string{"BLDR_IS_BROWSER": "true"},
			Loaders: map[string]string{
				".wasm": "asset", ".woff": "asset", ".woff2": "asset",
				".png": "asset", ".jpg": "asset", ".jpeg": "asset",
				".svg": "asset", ".gif": "asset",
			},
		},
	)
	if err != nil {
		return err
	}
	if result.GetEntrypointOutputs()["runtime-ws"] != "runtime-ws.mjs" {
		return errors.Errorf("WebSocket runtime output is %q", result.GetEntrypointOutputs()["runtime-ws"])
	}
	return nil
}
