package devtool

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	entrypoint_browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	web_runtime_controller "github.com/aperturerobotics/bldr/web/runtime/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
	"nhooyr.io/websocket"
)

// DevtoolWsVersion is the version to report for the ws-backed devtool runtime.
var DevtoolWsVersion = semver.MustParse("0.0.1")

// ExecuteWebWsProject starts the devtool bus and project as a web server with a
// WebSocket. Plugins run as native binaries under the devtool process.
func (a *DevtoolArgs) ExecuteWebWsProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	_ = repoRoot
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	buildType := bldr_manifest.BuildType(a.BuildType)
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath); err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// build the plugin host controller
	// TODO: re-enable this but make sure web plugin does not start electron
	/*
			_, relPluginHost, err := plugin_host_default.StartBusPluginHost(
				ctx,
				b.GetBus(),
				b.GetWorldEngineID(),
				b.GetPluginHostObjectKey(),
				b.GetVolume().GetID(),
				b.GetVolume().GetPeerID().String(),
				b.GetPluginsStateRoot(),
				b.GetPluginsDistRoot(),
		        "",
			)
			if err != nil {
				return err
			}
			if relPluginHost != nil {
				defer relPluginHost()
			}
	*/

	// execute the project controller
	_, projCtrlRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		a.Remote,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	return b.ExecuteWebWs(ctx, repoRoot, a.MinifyEntrypoint, buildType.IsDev(), a.WebListenAddr)
}

// ExecuteWebWs starts the application in the browser with a websocket.
func (b *DevtoolBus) ExecuteWebWs(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint, devMode bool,
	listenAddr string,
) error {
	le := b.GetLogger()
	stateDir := b.GetStateRoot()
	distSrcDir := b.GetDistSrcDir()
	entrypointDataDir := filepath.Join(stateDir, "entry")
	entrypointDir := filepath.Join(entrypointDataDir, "web/ws")

	// run esbuild to compile the web entrypoint
	le.Info("building websocket entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err := entrypoint_browser_bundle.BuildBrowserBundle(
		ctx,
		le,
		distSrcDir,
		entrypointDir,
		"./entrypoint/runtime-ws.mjs",
		minifyEntrypoint,
		devMode,
	)
	if err != nil {
		return err
	}

	// compile the entrypoint
	wsRuntimeDir := filepath.Join(entrypointDir, "entrypoint")
	if err := os.MkdirAll(wsRuntimeDir, 0o755); err != nil {
		return err
	}
	if err := entrypoint_browser_build.BuildWsRuntime(ctx, le, distSrcDir, wsRuntimeDir, minifyEntrypoint); err != nil {
		return err
	}

	// serve the entrypoint
	entryFs := http.Dir(entrypointDir)
	entrySrv := http.FileServer(entryFs)

	// start the local WebRuntime which communicates via WebSocket w/ the remote
	runtimeID := "devtool"

	// serve the websocket if the path matches
	webRuntimeWsPath := "/bldr-dev/web-runtime.ws"
	serveFn := func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == webRuntimeWsPath {
			le.Info("received websocket connection from frontend")
			wc, err := websocket.Accept(rw, req, &websocket.AcceptOptions{})
			if err != nil {
				le.WithError(err).Warn("unable to accept websocket conn")
				rw.WriteHeader(500)
				_, _ = rw.Write([]byte(err.Error()))
				return
			}
			ctrl := buildWsWebRuntime(le, b.GetBus(), runtimeID, wc)
			err = b.GetBus().ExecuteController(req.Context(), ctrl)
			if err != nil && err != context.Canceled && err != io.EOF {
				le.WithError(err).Warn("websocket disconnected with error")
			} else {
				le.Debug("websocket disconnected normally")
			}
			return
		}

		entrySrv.ServeHTTP(rw, req)
	}

	le.Infof("listening on: %s", listenAddr)
	return http.ListenAndServe(listenAddr, http.HandlerFunc(serveFn))
}

// buildWsWebRuntime builds a websocket web runtime controller.
func buildWsWebRuntime(le *logrus.Entry, b bus.Bus, runtimeID string, nch *websocket.Conn) *web_runtime_controller.Controller {
	return web_runtime_controller.NewController(
		le,
		b,
		func(
			ctx context.Context,
			le *logrus.Entry,
			handler web_runtime.WebRuntimeHandler,
		) (web_runtime.WebRuntime, error) {
			// mc must be a MuxedConn
			yamuxConf := srpc.NewYamuxConfig()
			yamuxConf.EnableKeepAlive = false
			yamuxConf.MaxMessageSize = 32 * 1024

			mc, err := srpc.NewWebSocketConn(ctx, nch, false, yamuxConf)
			if err != nil {
				return nil, err
			}
			rpcClient := srpc.NewClientWithMuxedConn(mc)
			return web_runtime.NewRemote(
				le,
				b,
				handler,
				runtimeID,
				rpcClient,
				func(ctx context.Context, r *web_runtime.Remote) error {
					return r.GetRpcServer().AcceptMuxedConn(ctx, mc)
				},
			)
		},
		runtimeID,
		DevtoolWsVersion,
	)
}
