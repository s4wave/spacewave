package browser_build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/aperturerobotics/bldr/util/gocompiler"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	"github.com/aperturerobotics/util/exec"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// webEntrypointBrowserDir is the repo sub-dir for the browser entrypoint.
var webEntrypointBrowserDir = "web/entrypoint/browser"

// BuildWasmRuntime builds the Wasm runtime entrypoint.
//
// builds to buildDir/runtime.wasm and buildDir/runtime-wasm.js
func BuildWasmRuntime(ctx context.Context, le *logrus.Entry, repoRoot, buildDir string, minify bool) error {
	le.Info("building runtime-wasm.js")
	goRootDir := runtime.GOROOT()
	wasmExecFile := filepath.Join(goRootDir, "misc/wasm/wasm_exec.js")
	if _, err := os.Stat(wasmExecFile); err != nil {
		return errors.Wrapf(err, "cannot find wasm_exec.js in goroot: %s", wasmExecFile)
	}

	entrypointDir := filepath.Join(repoRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-wasm.js")

	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointDir, minify)
	opts.EntryPoints = []string{"runtime-wasm.ts"}
	opts.Inject = append(opts.Inject, wasmExecFile)
	opts.Outfile = runtimeJsOut
	opts.Write = true

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild.BuildResultToErr(res); err != nil {
		return err
	}

	// get the bldr go mod
	bldrGoMod := "github.com/aperturerobotics/bldr"
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		bldrGoMod = buildInfo.Main.Path
	}
	entrypointPkg := bldrGoMod + "/" + webEntrypointBrowserDir

	le.Info("building runtime.wasm")
	runtimeOut := filepath.Join(buildDir, "runtime.wasm")
	goArgs := append([]string{
		"build",
		"-ldflags", "-s -w",
		"-o",
		runtimeOut,
	}, gocompiler.GetDefaultArgs()...)
	goArgs = append(goArgs, entrypointPkg)

	cmpCmd := gocompiler.NewGoCompilerCmd(goArgs...)
	cmpCmd.Env = append(cmpCmd.Env, "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmpCmd.Dir = entrypointDir
	if err := exec.StartAndWait(ctx, le, cmpCmd); err != nil {
		return err
	}

	// build complete
	return nil
}

// BuildWsRuntime builds the WebSocket dev runtime entrypoint.
//
// builds to buildDir/runtime-ws.js
func BuildWsRuntime(ctx context.Context, le *logrus.Entry, repoRoot, buildDir string, minify bool) error {
	le.Info("building runtime-ws.js")
	entrypointDir := filepath.Join(repoRoot, webEntrypointBrowserDir)
	runtimeJsOut := filepath.Join(buildDir, "runtime-ws.js")

	opts := entrypoint_browser_bundle.BrowserBuildOpts(entrypointDir, minify)
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
