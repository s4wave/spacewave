//go:build !js

package devtool

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	entrypoint_browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	web_runtime_controller "github.com/aperturerobotics/bldr/web/runtime/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/coder/websocket"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
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

	// start the plugin storage volume
	pluginVolumeID := bldr_plugin.PluginVolumeID
	_, pluginStorageCtrlRef, err := b.StartStorageVolume(ctx, "plugins", &volume_controller.Config{
		VolumeIdAlias: []string{bldr_plugin.PluginVolumeID},
	})
	if err != nil {
		return err
	}
	defer pluginStorageCtrlRef.Release()

	// HACK: set an environment variable to make the web plugin skip starting.
	// HACK: in future we can pass this via some other kind of signal.
	os.Setenv("BLDR_PLUGIN_WEB_SKIP_ELECTRON", "true")

	// build the plugin host scheduler
	_, relPluginSched, err := plugin_host_default.StartPluginScheduler(
		ctx,
		b.GetBus(),
		b.GetWorldEngineID(),
		b.GetPluginHostObjectKey(),
		pluginVolumeID,
		b.GetVolume().GetPeerID().String(),
		true,
		true,
		true,
	)
	if err != nil {
		return err
	}
	if relPluginSched != nil {
		defer relPluginSched()
	}

	// build the plugin host controller
	_, relPluginHost, err := plugin_host_default.StartPluginHost(
		ctx,
		b.GetBus(),
		b.GetPluginsStateRoot(),
		b.GetPluginsDistRoot(),
		"", // ignored on native platform
	)
	if err != nil {
		return err
	}
	if relPluginHost != nil {
		defer relPluginHost()
	}

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

	// entrypoint is located under /entrypoint/pkgs/@aperture/bldr
	entrypointToRootPrefix := "../../../../"

	// TODO: set webStartupSrcPath to control the root component in the WebView.
	var webStartupSrcPath string

	// run esbuild to compile the web entrypoint
	le.Info("building websocket entrypoint")
	entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err := entrypoint_browser_bundle.BuildBrowserBundle(
		ctx,
		le,
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
	server := &http.Server{Addr: listenAddr, Handler: http.HandlerFunc(serveFn), ReadHeaderTimeout: time.Second * 30}
	return server.ListenAndServe()
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
