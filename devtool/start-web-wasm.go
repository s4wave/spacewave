//go:build !js

package devtool

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	link_holdopen_controller "github.com/aperturerobotics/bifrost/link/hold-open"
	"github.com/aperturerobotics/bifrost/protocol"
	stream_srpc_server "github.com/aperturerobotics/bifrost/stream/srpc/server"
	transport_controller "github.com/aperturerobotics/bifrost/transport/controller"
	transport_websocket "github.com/aperturerobotics/bifrost/transport/websocket"
	devtool_web "github.com/aperturerobotics/bldr/devtool/web"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	entrypoint_browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
)

// ExecuteWebWasmProject starts the project as a web server in Wasm mode.
func (a *DevtoolArgs) ExecuteWebWasmProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	d, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer d.Release()

	if err := d.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath); err != nil {
		return err
	}

	// execute the project controller
	projCtrl, projCtrlRef, err := d.StartProjectController(
		ctx,
		d.GetBus(),
		repoRoot,
		a.ConfigPath,
		a.Remote,
		a.StartPlugins.Value(),
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	currProjCtrl, err := projCtrl.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// TODO: reload these if ProjectController restarts?
	currProjConf := currProjCtrl.GetConfig().GetProjectConfig()
	appID := currProjConf.GetId()
	startConf := currProjConf.GetStart()
	startupPlugins := startConf.GetPlugins()
	webStartupSrcPath, _ := startConf.ParseWebStartupPath()

	buildType := bldr_manifest.BuildType(a.BuildType)
	return d.ExecuteWebWasm(
		ctx,
		repoRoot,
		a.MinifyEntrypoint,
		buildType.IsDev(),
		a.WebListenAddr,
		appID,
		startupPlugins,
		webStartupSrcPath,
		false,
	)
}

// ExecuteWebWasm starts the application in the browser with wasm.
func (d *DevtoolBus) ExecuteWebWasm(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint,
	devMode bool,
	listenAddr string,
	appID string,
	startPlugins []string,
	webStartupSrcPath string,
	forceDedicatedWorkers bool,
) error {
	le := d.GetLogger()
	stateDir := d.GetStateRoot()
	distSrcDir := d.GetDistSrcDir()
	entrypointDataDir := filepath.Join(stateDir, "entry")
	entrypointDir := filepath.Join(entrypointDataDir, "web/wasm")

	// entrypoint is located under /entrypoint/pkgs/@aperture/bldr
	entrypointToRootPrefix := "../../../../"

	// run esbuild to compile the web entrypoint
	le.Info("building web wasm entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
	_, err := entrypoint_browser_bundle.BuildBrowserBundle(
		ctx,
		le,
		stateDir,
		repoRoot,
		distSrcDir,
		entrypointDir,
		// web-document is located under /pkgs/@aptre/bldr
		entrypointToRootPrefix+"entrypoint/runtime-wasm.mjs",
		entrypointToRootPrefix+"sw.mjs",
		entrypointToRootPrefix+"shw.mjs",
		webStartupSrcPath,
		"",
		minifyEntrypoint,
		devMode,
		forceDedicatedWorkers,
	)
	if err != nil {
		return err
	}

	// set the path to the entrypoint to use for the wasm main() function
	entrypointPkg := "devtool/web/entrypoint"

	// compile the entrypoint wasm
	buildPlatform, err := bldr_platform.ParseNativePlatform("web/js/wasm")
	if err != nil {
		return err
	}

	entryBuildType := bldr_manifest.BuildType_DEV
	if minifyEntrypoint {
		entryBuildType = bldr_manifest.BuildType_RELEASE
	}

	// disable tinygo unless release mode
	// NOTE: we disable tinygo since it does not compile cleanly yet.
	tinygoCompatible := false
	useTinygo := entryBuildType.IsRelease() && minifyEntrypoint && tinygoCompatible

	wasmRuntimeDir := filepath.Join(entrypointDir, "entrypoint")
	if err := os.MkdirAll(wasmRuntimeDir, 0o755); err != nil {
		return err
	}

	// start the websocket transport for the devtool
	linkWsPath := "/bldr-dev/web-wasm/link.ws"
	infoPath := "/bldr-dev/web-wasm/info"
	wsPeerID := d.peerID.String()
	wsCtrl, _, wsRef, err := loader.WaitExecControllerRunning(
		ctx,
		d.GetBus(),
		resolver.NewLoadControllerWithConfig(&transport_websocket.Config{
			TransportPeerId: wsPeerID,
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer wsRef.Release()

	wsTpt := wsCtrl.(*transport_controller.Controller)
	tpt, err := wsTpt.GetTransport(ctx)
	if err != nil {
		return err
	}
	ws := tpt.(*transport_websocket.WebSocket)

	// start the hold open controller to keep links open
	d.GetStaticResolver().AddFactory(link_holdopen_controller.NewFactory(d.GetBus()))
	_, _, holdOpenRef, err := loader.WaitExecControllerRunning(
		ctx,
		d.GetBus(),
		resolver.NewLoadControllerWithConfig(&link_holdopen_controller.Config{}),
		nil,
	)
	if err != nil {
		return err
	}
	defer holdOpenRef.Release()

	// handle incoming srpc requests
	rpcServer, err := stream_srpc_server.NewServer(
		d.GetBus(),
		le,
		controller.NewInfo(
			"devtool/web/rpc-server",
			semver.MustParse("0.0.1"),
			"listens for incoming requests from the web frontend",
		),
		[]stream_srpc_server.RegisterFn{
			// handle ManifestFetch requests via bus ManifestFetch.
			func(mux srpc.Mux) error {
				pluginFetchViaBus := bldr_manifest.NewManifestFetchViaBus(le, d.GetBus())
				return bldr_manifest.SRPCRegisterManifestFetch(mux, pluginFetchViaBus)
			},
			func(mux srpc.Mux) error {
				// proxy the devtool host volume via RPC
				proxyVol := volume_rpc_server.NewProxyVolume(ctx, d.GetVolume(), false)
				return volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, proxyVol, devtool_web.HostVolumeServiceIDPrefix)
			},
		},
		[]protocol.ID{devtool_web.HostProtocolID},
		[]string{wsPeerID},
		false,
	)
	if err != nil {
		return err
	}

	// start handling incoming srpc requests
	relRpcServer, err := d.GetBus().AddController(ctx, rpcServer, nil)
	if err != nil {
		return err
	}
	defer relRpcServer()

	// trigger FetchManifest for the startup plugins in advance
	// if this is commented out, the plugin build begins once the browser asks for it.
	if devMode {
		buildType := bldr_manifest.BuildType_DEV
		for _, startPluginID := range startPlugins {
			_, dir, err := d.GetBus().AddDirective(
				bldr_manifest.NewFetchManifest(
					startPluginID,
					[]bldr_manifest.BuildType{buildType},
					[]string{buildPlatform.GetPlatformID()},
					0,
				),
				nil,
			)
			if err != nil {
				return err
			}
			defer dir.Release()
		}
	}

	// build the wasm entrypooints concurrently with the plugins for speedup
	if err := entrypoint_browser_build.BuildWasmRuntimeEntrypoint(
		ctx,
		le,
		distSrcDir,
		wasmRuntimeDir,
		entryBuildType,
		useTinygo,
		"./runtime.wasm",
	); err != nil {
		return err
	}

	// Build runtime wasm pkg
	le.Info("building runtime.wasm")
	entrypointGoDir := filepath.Join(distSrcDir, entrypointPkg)
	runtimeOut := filepath.Join(wasmRuntimeDir, "runtime.wasm")
	if err := gocompiler.ExecBuildEntrypoint(
		ctx,
		le,
		buildPlatform,
		entryBuildType,
		entrypointGoDir,
		runtimeOut,
		false, // disable cgo
		useTinygo,
		nil,
		nil,
	); err != nil {
		return err
	}

	// encode the init info for the browser devtool entrypoint
	browserInit := &devtool_web.DevtoolInitBrowser{
		AppId:               appID,
		DevtoolPeerId:       wsPeerID,
		DevtoolVolumeInfo:   d.GetVolumeInfo(),
		StartPlugins:        startPlugins,
		ForceDedicatedWorkers: forceDedicatedWorkers,
	}
	if err := browserInit.Validate(); err != nil {
		return err
	}
	browserInitBin, err := browserInit.MarshalVT()
	if err != nil {
		return err
	}

	// run the http server
	entryFs := http.Dir(entrypointDir)
	entrySrv := http.FileServer(entryFs)

	serveFn := func(rw http.ResponseWriter, req *http.Request) {
		// Add Cross-Origin Isolation headers required for SharedArrayBuffer
		// These enable SAB-based communication between SharedWorkers
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		rw.Header().Set("Cross-Origin-Resource-Policy", "same-origin")

		if req.URL.Path == infoPath {
			le.Info("received info request from frontend")
			rw.Header().Add("Content-Type", "application/x-protobuf")
			rw.Header().Add("Content-Length", strconv.Itoa(len(browserInitBin)))
			rw.WriteHeader(200)
			_, _ = rw.Write(browserInitBin)
			return
		}
		if req.URL.Path == linkWsPath {
			le.Info("received websocket connection from frontend")
			ws.ServeHTTP(rw, req)
			return
		}

		entrySrv.ServeHTTP(rw, req)
	}

	// write the banner
	writeBanner()

	le.Infof("listening on: %s", listenAddr)
	server := &http.Server{Addr: listenAddr, Handler: http.HandlerFunc(serveFn), ReadHeaderTimeout: time.Second * 30}

	// Shut down the server when the context is cancelled.
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
