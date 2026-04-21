//go:build !js

package web_runtime_wasm_build

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// webRuntimeWasmDir is the repo sub-dir for the web runtime wasm entrypoint.
const webRuntimeWasmDir = "web/runtime/wasm"

// nodeStubsPath is the repo sub-dir for the node stubs
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
func BuildWebWasmPluginScript(ctx context.Context, le *logrus.Entry, bldrDistRoot, outPath, entrypointPath string, useTinygo, minify bool) error {
	if !HasValidWasmExtension(entrypointPath) {
		if entrypointPath == "" {
			entrypointPath = "<empty>"
		}
		return errors.Errorf("plugin-wasm: entrypoint path must end in %s: %s", strings.Join(validWasmSuffixes, " or "), entrypointPath)
	}

	wasmExecFile, err := gocompiler.GetWasmExecPath(ctx, le, useTinygo)
	if err != nil {
		return err
	}

	le.Infof("building plugin-wasm.ts to %v", filepath.Base(outPath))

	pluginJsDir := filepath.Join(bldrDistRoot, webRuntimeWasmDir)
	opts := entrypoint_browser_bundle.BrowserBuildOpts(pluginJsDir, minify)
	opts.EntryPoints = []string{"plugin-wasm.ts"}
	opts.Outfile = outPath
	opts.Define["BLDR_IS_PLUGIN"] = "true"
	opts.Define["BLDR_PLUGIN_ENTRYPOINT"] = strconv.Quote(entrypointPath)
	opts.Write = true

	if useTinygo {
		nodeStubsLoc := filepath.Join(bldrDistRoot, nodeStubsPath)
		nodeStubsLoc, err = filepath.Rel(pluginJsDir, nodeStubsLoc)
		if err != nil {
			return err
		}
		opts.Inject = append(opts.Inject, nodeStubsLoc)
	}

	opts.Inject = append(opts.Inject, wasmExecFile)

	if !minify {
		opts.Sourcemap = esbuild_api.SourceMapInlineAndExternal
	}

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}
