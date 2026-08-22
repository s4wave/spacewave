//go:build !js

package devtool

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	devtool_web "github.com/s4wave/spacewave/bldr/devtool/web"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	entrypoint_browser_build "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	web_plugin_compiler "github.com/s4wave/spacewave/bldr/web/plugin/compiler"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	web_runtime_controller "github.com/s4wave/spacewave/bldr/web/runtime/controller"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// errWebSocketConnectionsClosed is returned when a connection arrives after
// the server stopped accepting upgraded connections.
var errWebSocketConnectionsClosed = errors.New("devtool websocket connections closed")

// webSocketControllerConnections binds upgraded connections and their controllers to the server lifetime.
type webSocketControllerConnections struct {
	ctx context.Context

	mtx         sync.Mutex
	connections map[*websocket.Conn]struct{}
	closed      bool
	wait        sync.WaitGroup
}

func newWebSocketControllerConnections(ctx context.Context) *webSocketControllerConnections {
	return &webSocketControllerConnections{ctx: ctx, connections: make(map[*websocket.Conn]struct{})}
}

func (c *webSocketControllerConnections) Execute(connection *websocket.Conn, execute func(context.Context) error) error {
	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		_ = connection.CloseNow()
		return errWebSocketConnectionsClosed
	}
	if ctxErr := c.ctx.Err(); ctxErr != nil {
		c.mtx.Unlock()
		_ = connection.CloseNow()
		return ctxErr
	}
	c.connections[connection] = struct{}{}
	c.wait.Add(1)
	c.mtx.Unlock()
	defer func() {
		c.mtx.Lock()
		delete(c.connections, connection)
		c.mtx.Unlock()
		c.wait.Done()
	}()
	return execute(c.ctx)
}

func (c *webSocketControllerConnections) Close() {
	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return
	}
	c.closed = true
	connections := make([]*websocket.Conn, 0, len(c.connections))
	for connection := range c.connections {
		connections = append(connections, connection)
	}
	c.mtx.Unlock()
	for _, connection := range connections {
		_ = connection.CloseNow()
	}
}

func (c *webSocketControllerConnections) Wait() { c.wait.Wait() }

// DevtoolWsVersion is the version to report for the ws-backed devtool runtime.
var DevtoolWsVersion = controller.MustParseVersion("0.0.1")

// ExecuteWebWsProject starts the devtool bus and project as a web server with a
// WebSocket. Plugins run as native binaries under the devtool process.
func (a *DevtoolArgs) ExecuteWebWsProject(ctx context.Context) (err error) {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	buildType := bldr_manifest.BuildType(a.BuildType)
	d, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer d.Release()
	commandLogFile := a.commandLogFile()
	d.setCommandStartingWithLogFile("start web", "initializing web runtime", commandLogFile)
	ctx, stopTUI := a.startDevtoolTUI(ctx, d.GetStatusProducer(), "http://"+a.WebListenAddr)
	defer func() {
		d.finishCommandWithLogFile(ctx, "start web", commandLogFile, err)
		stopTUI()
	}()

	err = d.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath)
	if err != nil {
		return err
	}

	// write the banner
	a.writeBannerTo(os.Stderr)

	// start the plugin storage volume
	pluginVolumeID := bldr_plugin.PluginVolumeID
	_, pluginStorageCtrlRef, err := d.StartStorageVolume(ctx, "plugins", &volume_controller.Config{
		VolumeIdAlias: []string{bldr_plugin.PluginVolumeID},
	})
	if err != nil {
		return err
	}
	defer pluginStorageCtrlRef.Release()

	// Web-server mode still loads the web plugin so it registers its packages
	// and RPCs, but the devtool serves the browser shell and must not embed a
	// native renderer.
	if err := os.Setenv(web_plugin_compiler.SkipNativeWebRendererEnvVar, "true"); err != nil {
		return err
	}

	// execute the project controller
	projCtrl, projCtrlRef, err := d.StartProjectControllerWithStartup(
		ctx,
		d.GetBus(),
		repoRoot,
		a.ConfigPath,
		a.Remote,
		a.StartPlugins.Value(),
		false,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	currProjCtrl, err := projCtrl.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	projectConfig := currProjCtrl.GetConfig().GetProjectConfig()
	webStartupSrcPath, _ := projectConfig.GetStart().ParseWebStartupPath()
	preflightRemote := a.Remote
	if preflightRemote == "" {
		preflightRemote = "devtool"
	}
	startPlugins := projectOwnedStartupPlugins(projectConfig)
	if len(startPlugins) != 0 {
		le.WithField("plugin-count", len(startPlugins)).Info("preflighting startup manifests")
		_, _, err = currProjCtrl.BuildManifests(
			ctx,
			preflightRemote,
			startPlugins,
			bldr_manifest.BuildType(a.BuildType),
			nil,
		)
		if err != nil {
			return err
		}
	}

	// build the plugin host scheduler
	sched, relPluginSched, err := plugin_host_default.StartPluginScheduler(
		ctx,
		d.GetBus(),
		"",
		d.GetWorldEngineID(),
		d.GetPluginHostObjectKey(),
		pluginVolumeID,
		d.GetVolume().GetPeerID().String(),
		true,
		true,
		true,
	)
	if err != nil {
		return err
	}
	devtool_status.AttachPluginStatus(ctx, d.GetStatusProducer(), sched)
	if relPluginSched != nil {
		defer relPluginSched()
	}

	// build the plugin host controller
	_, relPluginHost, err := plugin_host_default.StartPluginHost(
		ctx,
		d.GetBus(),
		d.GetPluginsStateRoot(),
		d.GetPluginsDistRoot(),
		"", // ignored on native platform
	)
	if err != nil {
		return err
	}
	if relPluginHost != nil {
		defer relPluginHost()
	}

	currProjCtrl.StartStartup(ctx)

	statusMux := srpc.NewMux()
	if err := devtool_status.RegisterDevtoolStatusService(statusMux, d.GetStatusProducer()); err != nil {
		return err
	}
	statusClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(statusMux)))
	statusCtrl := bifrost_rpc.NewClientController(
		le,
		d.GetBus(),
		controller.NewInfo(
			"bldr/devtool/status/web-ws-rpc",
			DevtoolWsVersion,
			"bldr devtool status web-ws rpc",
		),
		statusClient,
		[]string{devtool_web.HostServiceIDPrefix},
	)
	relStatusCtrl, err := d.GetBus().AddController(ctx, statusCtrl, nil)
	if err != nil {
		return err
	}
	defer relStatusCtrl()

	return d.executeWebWs(ctx, repoRoot, a.MinifyEntrypoint, buildType.IsDev(), a.WebListenAddr, webStartupSrcPath, func(string) error {
		d.setCommandRunningWithLogFile("start web", "web runtime active on "+a.WebListenAddr, commandLogFile)
		return nil
	})
}

func (d *DevtoolBus) executeWebWs(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint, devMode bool,
	listenAddr string,
	webStartupSrcPath string,
	onListening func(string) error,
) error {
	le := d.GetLogger()
	stateDir := d.GetStateRoot()
	distSrcDir := d.GetDistSrcDir()
	entrypointDataDir := filepath.Join(stateDir, "entry")
	entrypointDir := filepath.Join(entrypointDataDir, "web/ws")

	// entrypoint is located under /entrypoint/pkgs/@aperture/bldr
	entrypointToRootPrefix := "../../../../"

	// Compile the web entrypoint.
	le.Info("building websocket entrypoint")
	bundleResult, err := entrypoint_browser_bundle.BuildBrowserBundle(
		ctx,
		le,
		stateDir,
		repoRoot,
		distSrcDir,
		entrypointDir,
		// web-document is located under /pkgs/@aptre/bldr
		entrypointToRootPrefix+"entrypoint/runtime-ws.mjs",
		entrypointToRootPrefix+"sw.mjs",
		entrypointToRootPrefix+"shw.mjs",
		webStartupSrcPath,
		"",
		minifyEntrypoint,
		!minifyEntrypoint,
		devMode,
		false,
		false,
	)
	if err != nil {
		return err
	}

	// compile the entrypoint
	wsRuntimeDir := filepath.Join(entrypointDir, "entrypoint")
	if err := os.MkdirAll(wsRuntimeDir, 0o755); err != nil {
		return err
	}
	if err := entrypoint_browser_build.BuildWsRuntime(ctx, le, distSrcDir, wsRuntimeDir, minifyEntrypoint, !minifyEntrypoint); err != nil {
		return err
	}
	if err := writeWebWsBuildManifest(entrypointDir, bundleResult); err != nil {
		return err
	}

	// serve the entrypoint
	entryFs := http.Dir(entrypointDir)
	entrySrv := bifrost_http.NewEncodedAssetFileServer(entryFs)

	// start the local WebRuntime which communicates via WebSocket w/ the remote
	runtimeID := "devtool"

	// serve the websocket if the path matches
	webRuntimeWsPath := "/bldr-dev/web-runtime.ws"
	connections := newWebSocketControllerConnections(ctx)
	serveFn := func(rw http.ResponseWriter, req *http.Request) {
		// Add Cross-Origin Isolation headers required for SharedArrayBuffer
		// These enable SAB-based communication between SharedWorkers
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")

		if req.URL.Path == webRuntimeWsPath {
			le.Info("received websocket connection from frontend")
			wc, err := websocket.Accept(rw, req, &websocket.AcceptOptions{})
			if err != nil {
				le.WithError(err).Warn("unable to accept websocket conn")
				http.Error(rw, "unable to accept websocket connection", http.StatusInternalServerError)
				return
			}
			ctrl := buildWsWebRuntime(le, d.GetBus(), runtimeID, wc)
			err = connections.Execute(wc, func(connectionCtx context.Context) error {
				return d.GetBus().ExecuteController(connectionCtx, ctrl)
			})
			if err != nil &&
				!errors.Is(err, context.Canceled) &&
				!errors.Is(err, io.EOF) &&
				!errors.Is(err, errWebSocketConnectionsClosed) {
				le.WithError(err).Warn("websocket disconnected with error")
			} else {
				le.Debug("websocket disconnected normally")
			}
			return
		}

		entrySrv.ServeHTTP(rw, req)
	}

	le.Infof("listening on: %s", listenAddr)
	server := &http.Server{Addr: listenAddr, Handler: http.HandlerFunc(serveFn), ReadHeaderTimeout: time.Second * 30}
	return listenAndServeDevtoolWebWs(ctx, server, connections, onListening)
}

// listenAndServeDevtoolWebWs serves the devtool websocket HTTP server until it
// returns, then closes every upgraded connection and waits for its controller
// to exit before returning.
func listenAndServeDevtoolWebWs(
	ctx context.Context,
	server *http.Server,
	connections *webSocketControllerConnections,
	onListening func(string) error,
) error {
	err := listenAndServeDevtoolHTTP(ctx, server, onListening)
	connections.Close()
	connections.Wait()
	return err
}

func writeWebWsBuildManifest(entrypointDir string, bundleResult *entrypoint_browser_bundle.BrowserBundleResult) error {
	return entrypoint_browser_bundle.WriteBuildManifest(entrypointDir, &entrypoint_browser_bundle.BuildManifest{
		Entrypoint:                 bundleResult.EntrypointPath,
		EntrypointDecompressedSize: bundleResult.EntrypointDecompressedSize,
		ServiceWorker:              bundleResult.ServiceWorkerFilename,
		SharedWorker:               bundleResult.SharedWorkerFilename,
		OpfsWorker:                 bundleResult.OpfsWorkerFilename,
		// The websocket dev runtime has no runtime.wasm. boot.mjs still
		// preloads the shellAssets.wasm path, so point it at the runtime asset
		// that this server actually serves.
		Wasm: "entrypoint/runtime-ws.mjs",
		CSS:  bundleResult.CSSPaths,
	})
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

func listenAndServeDevtoolHTTP(ctx context.Context, server *http.Server, onListening func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- server.Serve(listener) }()
	stopShutdownCh := make(chan struct{})
	shutdownErrCh := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			// Drain plain HTTP requests without a deadline. Upgraded
			// (hijacked) connections are not tracked by Shutdown; their
			// lifecycle owner closes and joins them separately.
			shutdownErrCh <- server.Shutdown(context.WithoutCancel(ctx))
		case <-stopShutdownCh:
			shutdownErrCh <- nil
		}
	}()

	var callbackErr error
	if onListening != nil {
		callbackErr = onListening(listener.Addr().String())
		if callbackErr != nil {
			_ = server.Close()
		}
	}
	serveErr := <-serveErrCh
	close(stopShutdownCh)
	shutdownErr := <-shutdownErrCh
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(callbackErr, serveErr, shutdownErr)
}
