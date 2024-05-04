package web_runtime_wasm_build

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/bldr/util/gocompiler"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/esbuild/build"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// webRuntimeWasmDir is the repo sub-dir for the web runtime wasm entrypoint.
const webRuntimeWasmDir = "web/runtime/wasm"

// nodeStubsPath is the repo sub-dir for the node stubs
const nodeStubsPath = "web/runtime/wasm/node-stubs.js"

// BuildWebWasmPluginScript builds the web plugin runtime entrypoint script.
//
// outPath should have a .mjs suffix
// entrypointPath should be foo.wasm (relative to script location)
func BuildWebWasmPluginScript(ctx context.Context, le *logrus.Entry, bldrDistRoot, outPath, entrypointPath string, useTinygo, minify bool) error {
	if !strings.HasSuffix(entrypointPath, ".wasm") && !strings.HasSuffix(entrypointPath, ".wasm.br") {
		if entrypointPath == "" {
			entrypointPath = "<empty>"
		}
		return errors.Errorf("plugin-wasm: entrypoint path must end in .wasm or .wasm.br: %s", entrypointPath)
	}

	wasmExecFile, err := gocompiler.GetWasmExecPath(le, useTinygo)
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
