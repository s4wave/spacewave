package devtool

import (
	"context"
	"net/http"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/banner"
	entrypoint_browser_build "github.com/aperturerobotics/bldr/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/entrypoint/browser/bundle"
	plugin_platform "github.com/aperturerobotics/bldr/plugin/platform"
	esbuild "github.com/evanw/esbuild/pkg/api"
	fcolor "github.com/fatih/color"
)

// ExecuteWebProject starts the project as a web server.
func (a *DevtoolArgs) ExecuteWebProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir)
	if err != nil {
		return err
	}
	defer b.Release()

	// execute the project controller
	_, projCtrlRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		true,
		repoRoot,
		a.ConfigPath,
		plugin_platform.PlatformID_GO_WASM_WEB,
		a.BuildType,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	return b.ExecuteWebWasm(ctx, repoRoot, a.MinifyEntrypoint, a.BldrVersion, a.BldrVersionSum, a.WebListenAddr)
}

// ExecuteWebWasm starts the application in the browser with wasm.
func (b *DevtoolBus) ExecuteWebWasm(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint bool,
	bldrVersion, bldrSum string,
	listenAddr string,
) error {
	if err := b.SyncWebSources(bldrVersion, bldrSum); err != nil {
		return err
	}

	le := b.GetLogger()
	stateDir := b.GetStateRoot()
	webSrcDir := b.GetWebSrcDir()
	entrypointDataDir := path.Join(stateDir, "entry")
	entrypointDir := path.Join(entrypointDataDir, "web")

	// run esbuild to compile the web entrypoint
	le.Info("building web entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err := entrypoint_browser_bundle.BuildBrowserBundle(le, webSrcDir, entrypointDir, minifyEntrypoint)
	if err != nil {
		return err
	}

	// compile the entrypoint wasm
	wasmRuntimeDir := path.Join(entrypointDir, "runtime")
	if err := os.MkdirAll(entrypointDir, 0755); err != nil {
		return err
	}
	if err := entrypoint_browser_build.BuildWasmRuntime(ctx, le, webSrcDir, wasmRuntimeDir); err != nil {
		return err
	}

	// write the banner
	red := fcolor.New(fcolor.FgRed)
	red.Fprint(os.Stderr, banner.FormatBanner()+"\n")

	// run the http server
	entryFs := http.Dir(entrypointDir)
	entrySrv := http.FileServer(entryFs)
	le.Infof("listening on: %s", listenAddr)
	return http.ListenAndServe(listenAddr, entrySrv)
}
