package devtool

import (
	"context"
	"net/http"
	"os"
	"path"

	entrypoint_browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	esbuild "github.com/evanw/esbuild/pkg/api"
)

// TODO: load plugins to the web wasm runtime

// ExecuteWebWasmProject starts the project as a web server in Wasm mode..
func (a *DevtoolArgs) ExecuteWebWasmProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum); err != nil {
		return err
	}

	// execute the project controller
	_, projCtrlRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		"", // a.Remote,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	return b.ExecuteWebWasm(ctx, repoRoot, a.MinifyEntrypoint, a.WebListenAddr)
}

// ExecuteWebWasm starts the application in the browser with wasm.
func (b *DevtoolBus) ExecuteWebWasm(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint bool,
	listenAddr string,
) error {
	le := b.GetLogger()
	stateDir := b.GetStateRoot()
	distSrcDir := b.GetDistSrcDir()
	entrypointDataDir := path.Join(stateDir, "entry")
	entrypointDir := path.Join(entrypointDataDir, "web/wasm")

	// run esbuild to compile the web entrypoint
	le.Info("building web wasm entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err := entrypoint_browser_bundle.BuildBrowserBundle(
		le,
		distSrcDir,
		entrypointDir,
		"/runtime/runtime-wasm.js",
		minifyEntrypoint,
	)
	if err != nil {
		return err
	}

	// compile the entrypoint wasm
	wasmRuntimeDir := path.Join(entrypointDir, "runtime")
	if err := os.MkdirAll(entrypointDir, 0755); err != nil {
		return err
	}
	if err := entrypoint_browser_build.BuildWasmRuntime(ctx, le, distSrcDir, wasmRuntimeDir); err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// run the http server
	entryFs := http.Dir(entrypointDir)
	entrySrv := http.FileServer(entryFs)
	le.Infof("listening on: %s", listenAddr)
	return http.ListenAndServe(listenAddr, entrySrv)
}
