//go:build !js

package web_runtime_wasm_build

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

// webRuntimeWasmDir is the repo sub-dir for the web runtime wasm entrypoint.
const webRuntimeWasmDir = "web/runtime/wasm"

// nodeStubsPath is the repo sub-dir for the node stubs.
const nodeStubsPath = "web/runtime/wasm/node-stubs.js"

// validWasmSuffixes are the set of allowed .wasm suffixes.
var validWasmSuffixes = []string{
	".wasm",
	// js decompression stream
	".wasm.gz",

	// go brotli decoder
	// NOTE: We do not bundle go-brotli-decoder currently.
	// See: github.com/s4wave/spacewave/db/unixfs/access/http/ext
	// This can be enabled if using the Ext version.
	// ".wasm.br",
}

// HasValidWasmExtension checks if the path has a valid wasm extension.
func HasValidWasmExtension(filePath string) bool {
	return slices.ContainsFunc(validWasmSuffixes, func(sfx string) bool {
		return strings.HasSuffix(filePath, sfx)
	})
}

// BuildWebWasmPluginScript builds the web plugin runtime entrypoint script.
//
// outPath should have a .mjs suffix
// entrypointPath should be foo.wasm (relative to script location)
func BuildWebWasmPluginScript(ctx context.Context, le *logrus.Entry, bldrDistRoot, outPath, entrypointPath string, useTinygo, minify, sourcemaps bool) ([]string, error) {
	if !HasValidWasmExtension(entrypointPath) {
		if entrypointPath == "" {
			entrypointPath = "<empty>"
		}
		return nil, errors.Errorf("plugin-wasm: entrypoint path must end in %s: %s", strings.Join(validWasmSuffixes, " or "), entrypointPath)
	}

	wasmExecFile, err := gocompiler.GetWasmExecPath(ctx, le, useTinygo)
	if err != nil {
		return nil, err
	}

	le.Infof("building plugin-wasm.ts to %v", filepath.Base(outPath))
	outputRoot := filepath.Dir(outPath)
	outputName := filepath.Base(outPath)
	entrypointName := strings.TrimSuffix(outputName, filepath.Ext(outputName))
	sourceMap := "none"
	if sourcemaps {
		sourceMap = "both"
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
			return nil, err
		}
		sourceOverrides = map[string]string{wasmExecFile: patched}
	}
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		outputRoot,
		bldrDistRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   outputRoot,
			SourceRoot:   bldrDistRoot,
			OutputRoot:   outputRoot,
			BldrDistRoot: bldrDistRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      entrypointName,
				InputPath: bldr.ResolveDistSourcePath(bldrDistRoot, webRuntimeWasmDir, "plugin-wasm.ts"),
			}},
			Format:         "es",
			Platform:       "browser",
			Target:         "es2024",
			EntryFileNames: outputName,
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      sourceMap,
			Minify:         minify,
			TreeShaking:    true,
			Banner:         entrypoint_browser_bundle.DefaultBanner()["js"],
			Defines: map[string]string{
				"BLDR_IS_BROWSER":        "true",
				"BLDR_IS_PLUGIN":         "true",
				"BLDR_PLUGIN_ENTRYPOINT": strconv.Quote(entrypointPath),
			},
			External:        external,
			Loaders:         map[string]string{".wasm": "asset"},
			Inject:          inject,
			SourceOverrides: sourceOverrides,
		},
	)
	if err != nil {
		return nil, err
	}
	if result.GetEntrypointOutputs()[entrypointName] != outputName {
		return nil, errors.Errorf("Wasm runtime output is %q, expected %q", result.GetEntrypointOutputs()[entrypointName], outputName)
	}
	return slices.Clone(result.GetInputs()), nil
}
