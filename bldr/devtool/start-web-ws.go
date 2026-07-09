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
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
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

	// Web-server mode still loads the web plugin for package/RPC ownership, but
	// the devtool serves the browser shell and must not embed a native renderer.
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

	if err := ctx.Err(); err != nil {
		return err
	}
	d.setCommandRunningWithLogFile("start web", "web runtime active on "+a.WebListenAddr, commandLogFile)
	return d.ExecuteWebWs(ctx, repoRoot, a.MinifyEntrypoint, buildType.IsDev(), a.WebListenAddr, webStartupSrcPath)
}

// ExecuteWebWs starts the application in the browser with a websocket.
func (d *DevtoolBus) ExecuteWebWs(
	ctx context.Context,
	repoRoot string,
	minifyEntrypoint, devMode bool,
	listenAddr string,
	webStartupSrcPath string,
) error {
	le := d.GetLogger()
	stateDir := d.GetStateRoot()
	distSrcDir := d.GetDistSrcDir()
	entrypointDataDir := filepath.Join(stateDir, "entry")
	entrypointDir := filepath.Join(entrypointDataDir, "web/ws")

	// entrypoint is located under /entrypoint/pkgs/@aperture/bldr
	entrypointToRootPrefix := "../../../../"

	// run esbuild to compile the web entrypoint
	le.Info("building websocket entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
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
			err = d.GetBus().ExecuteController(req.Context(), ctrl)
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
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
	return listenAndServeDevtoolHTTP(ctx, server, nil)
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
	go func() {
		serveErrCh <- server.Serve(listener)
	}()
	stopShutdownCh := make(chan struct{})
	shutdownDoneCh := make(chan struct{})
	defer func() {
		close(stopShutdownCh)
		<-shutdownDoneCh
	}()
	go func() {
		defer close(shutdownDoneCh)
		select {
		case <-ctx.Done():
			shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer shutdownCancel()
			_ = server.Shutdown(shutdownCtx)
		case <-stopShutdownCh:
		}
	}()

	if onListening != nil {
		if err := onListening(listener.Addr().String()); err != nil {
			_ = server.Close()
			<-serveErrCh
			return err
		}
	}
	err = <-serveErrCh
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
