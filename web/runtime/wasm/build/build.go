package web_runtime_wasm_build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// webRuntimeWasmDir is the repo sub-dir for the web runtime wasm entrypoint.
const webRuntimeWasmDir = "web/runtime/wasm"

// BuildWebWasmPluginScript builds the web plugin runtime entrypoint script.
//
// outPath should have a .mjs suffix
// entrypointPath should be foo.wasm (relative to script location)
func BuildWebWasmPluginScript(ctx context.Context, le *logrus.Entry, bldrDistRoot, outPath, entrypointPath string, minify bool) error {
	if !strings.HasSuffix(entrypointPath, ".wasm") {
		if entrypointPath == "" {
			entrypointPath = "<empty>"
		}
		return errors.Errorf("plugin-wasm: entrypoint path must end in .wasm: %s", entrypointPath)
	}

	goRootDir := runtime.GOROOT()
	wasmExecFile := filepath.Join(goRootDir, "misc/wasm/wasm_exec.js")
	if _, err := os.Stat(wasmExecFile); err != nil {
		return errors.Wrapf(err, "cannot find wasm_exec.js in goroot: %s", wasmExecFile)
	}

	le.Infof("building plugin-wasm.ts to %v", filepath.Base(outPath))

	pluginJsDir := filepath.Join(bldrDistRoot, webRuntimeWasmDir)
	opts := entrypoint_browser_bundle.BrowserBuildOpts(pluginJsDir, minify)
	opts.EntryPoints = []string{"plugin-wasm.ts"}
	opts.Outfile = outPath
	opts.Define["BLDR_IS_PLUGIN"] = "true"
	opts.Define["BLDR_PLUGIN_ENTRYPOINT"] = strconv.Quote(entrypointPath)
	opts.Inject = append(opts.Inject, wasmExecFile)
	opts.Write = true

	if !minify {
		opts.Sourcemap = esbuild_api.SourceMapInlineAndExternal
	}

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}
