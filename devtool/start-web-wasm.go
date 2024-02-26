package devtool

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	link_holdopen_controller "github.com/aperturerobotics/bifrost/link/hold-open"
	"github.com/aperturerobotics/bifrost/protocol"
	stream_srpc_server "github.com/aperturerobotics/bifrost/stream/srpc/server"
	transport_controller "github.com/aperturerobotics/bifrost/transport/controller"
	transport_websocket "github.com/aperturerobotics/bifrost/transport/websocket"
	devtool_web "github.com/aperturerobotics/bldr/devtool/web"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	entrypoint_browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	esbuild "github.com/evanw/esbuild/pkg/api"
)

// TODO: load plugins to the web wasm runtime

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
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch, false)
	if err != nil {
		return err
	}
	defer b.Release()

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum); err != nil {
		return err
	}

	// execute the project controller
	projCtrl, projCtrlRef, err := b.StartProjectController(
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

	currProjCtrl, err := projCtrl.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	appID := currProjCtrl.GetConfig().GetProjectConfig().GetId()

	buildType := bldr_manifest.BuildType(a.BuildType)
	return b.ExecuteWebWasm(
		ctx,
		repoRoot,
		a.MinifyEntrypoint,
		buildType.IsDev(),
		a.WebListenAddr,
		appID,
	)
}

// ExecuteWebWasm starts the application in the browser with wasm.
func (b *DevtoolBus) ExecuteWebWasm(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint,
	devMode bool,
	listenAddr string,
	appID string,
) error {
	le := b.GetLogger()
	stateDir := b.GetStateRoot()
	distSrcDir := b.GetDistSrcDir()
	entrypointDataDir := filepath.Join(stateDir, "entry")
	entrypointDir := filepath.Join(entrypointDataDir, "web/wasm")

	// run esbuild to compile the web entrypoint
	le.Info("building web wasm entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err := entrypoint_browser_bundle.BuildBrowserBundle(
		ctx,
		le,
		distSrcDir,
		entrypointDir,
		"/runtime/runtime-wasm.js",
		minifyEntrypoint,
		devMode,
	)
	if err != nil {
		return err
	}

	// get the bldr go mod
	entrypointPkg := "devtool/web/entrypoint"

	// compile the entrypoint wasm
	wasmRuntimeDir := filepath.Join(entrypointDir, "runtime")
	if err := os.MkdirAll(entrypointDir, 0755); err != nil {
		return err
	}
	if err := entrypoint_browser_build.BuildWasmRuntime(
		ctx,
		le,
		distSrcDir,
		wasmRuntimeDir,
		entrypointPkg,
		minifyEntrypoint,
	); err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// start the websocket transport for the devtool
	linkWsPath := "/bldr-dev/web-wasm/link.ws"
	infoPath := "/bldr-dev/web-wasm/info"
	wsPeerID := b.peerID.String()
	wsCtrl, _, wsRef, err := loader.WaitExecControllerRunning(
		ctx,
		b.GetBus(),
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
	_, _, holdOpenRef, err := loader.WaitExecControllerRunning(
		ctx,
		b.GetBus(),
		resolver.NewLoadControllerWithConfig(&link_holdopen_controller.Config{}),
		nil,
	)
	if err != nil {
		return err
	}
	defer holdOpenRef.Release()

	// handle incoming srpc requests
	rpcServer, err := stream_srpc_server.NewServer(
		b.GetBus(),
		le,
		controller.NewInfo(
			"devtool/web/rpc-server",
			semver.MustParse("0.0.1"),
			"listens for incoming requests from the web frontend",
		),
		[]stream_srpc_server.RegisterFn{
			// handle ManifestFetch requests via bus ManifestFetch.
			func(mux srpc.Mux) error {
				pluginFetchViaBus := bldr_manifest.NewManifestFetchViaBus(le, b.GetBus())
				return bldr_manifest.SRPCRegisterManifestFetch(mux, pluginFetchViaBus)
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
	relRpcServer, err := b.GetBus().AddController(ctx, rpcServer, nil)
	if err != nil {
		return err
	}
	defer relRpcServer()

	// encode the init info for the browser devtool entrypoint
	browserInitBin, err := (&devtool_web.DevtoolInitBrowser{
		AppId:         appID,
		DevtoolPeerId: wsPeerID,
	}).MarshalVT()
	if err != nil {
		return err
	}

	// run the http server
	entryFs := http.Dir(entrypointDir)
	entrySrv := http.FileServer(entryFs)
	le.Infof("listening on: %s", listenAddr)

	serveFn := func(rw http.ResponseWriter, req *http.Request) {
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

	return http.ListenAndServe(listenAddr, http.HandlerFunc(serveFn))
}
