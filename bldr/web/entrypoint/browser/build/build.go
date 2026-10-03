//go:build !js

package browser_build

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/distpath"
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
	// Expose browser runtime build progress to the caller.
	le.Info("building runtime-wasm.mjs")

	// Resolve the wasm execution shim for the selected compiler.
	wasmExecFile, err := gocompiler.GetWasmExecPath(ctx, le, useTinygo)
	if err != nil {
		return err
	}

	// Select the browser runtime output and source-map mode.
	runtimeJsOut := filepath.Join(buildDir, "runtime-wasm.mjs")
	sourceMap := "none"
	if sourcemaps {
		sourceMap = "external"
	}

	// Prepare compiler-specific execution shims and browser module overrides.
	inject := []string{wasmExecFile}
	var external []string
	var sourceOverrides map[string]string
	if useTinygo {
		// Supply browser stubs for the TinyGo execution shim dependencies.
		nodeStubsLoc := distpath.Resolve(bldrDistRoot, nodeStubsPath)
		inject = append([]string{nodeStubsLoc}, inject...)
		external = []string{"fs", "crypto", "util", "node:fs", "node:crypto", "node:util"}

		// Patch the TinyGo execution shim before bundling it for the browser.
		patched, err := entrypoint_browser_bundle.LoadTinyGoWasmExecSource(wasmExecFile)
		if err != nil {
			return err
		}
		sourceOverrides = map[string]string{wasmExecFile: patched}
	}

	// Bind the browser runtime location in the entrypoint source.
	defines := map[string]string{"BLDR_IS_BROWSER": "true"}
	if runtimeWasmPath != "" {
		defines["BLDR_RUNTIME_WASM"] = strconv.Quote(runtimeWasmPath)
	}

	// Bundle the browser runtime entrypoint with its compiler shim and overrides.
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
				InputPath: distpath.Resolve(bldrDistRoot, webEntrypointBrowserDir, "runtime-wasm.ts"),
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

	// Require the runtime entrypoint to use the expected output filename.
	if result.GetEntrypointOutputs()["runtime-wasm"] != filepath.Base(runtimeJsOut) {
		return errors.Errorf("Wasm runtime output is %q", result.GetEntrypointOutputs()["runtime-wasm"])
	}

	return nil
}
