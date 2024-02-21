package plugin_host_web

import (
	"context"
	"path/filepath"
	"time"

	"github.com/aperturerobotics/bifrost/util/randstring"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the process host controller ID.
const ControllerID = "bldr/plugin/host/web"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// WebHost implements the plugin host with WebWorker processes.
type WebHost struct {
	// le is the logger
	le *logrus.Entry
	// stateDir is the directory to use for state
	stateDir string
	// binsDir is the directory to use for binaries
	distDir string
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
}

// NewWebHost constructs a new WebHost.
func NewWebHost(le *logrus.Entry) (*WebHost, error) {
	// determine the platform id for the host
	platformID := (&bldr_platform.WebPlatform{}).GetPlatformID()
	return &WebHost{
		le:               le,
		pluginPlatformID: platformID,
	}, nil
}

// NewWebHostController constructs the WebHost and PluginHost controller.
func NewWebHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *Config,
) (*host_controller.Controller, *WebHost, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	processHost, err := NewWebHost(le)
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		c.GetHostConfig(),
		controller.NewInfo(ControllerID, Version, "plugin host with WebWorker processes"),
		processHost,
	)
	return hctrl, processHost, nil
}

// GetPlatformId returns the plugin platform ID for this host.
// Return empty if the host accepts any platform ID.
func (h *WebHost) GetPlatformId(ctx context.Context) (string, error) {
	return h.pluginPlatformID, nil
}

// ListPlugins lists the set of initialized plugins.
func (h *WebHost) ListPlugins(ctx context.Context) ([]string, error) {
	// TODO list stored plugins or temporary storage
	// the plugin host will call Delete for any unrecognized
	return nil, nil
}

// ExecutePlugin executes the plugin with the given ID.
// If the plugin was already initialized, existing state can be reused.
// The plugin should be stopped if/when the function exits.
// Return ErrPluginUninitialized if the plugin was not ready.
// Should expect to be called only once (at a time) for a plugin ID.
// pluginDist contains the plugin distribution files (binaries and assets).
func (h *WebHost) ExecutePlugin(
	rctx context.Context,
	pluginID, entrypoint string,
	pluginDist *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// double-check the entrypoint exists and is executable
	entrypoint = filepath.Clean(entrypoint)
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be an executable regular file: %s", entrypointFiMode.String())
	}

	// TODO mount the plugin dist unixfs to handle http requests
	/*
		if err := unixfs_sync.Sync(
			ctx,
			pluginDistDir,
			pluginDist,
			unixfs_sync.DeleteMode_DeleteMode_BEFORE,
			nil,
		); err != nil {
			return err
		}
	*/

	/*
		entrypointPath := filepath.Join(pluginDistDir, entrypoint)
		if err := os.Chmod(entrypointPath, 0755); err != nil {
			return err
		}
	*/

	// TODO configure entrypoint process
	// entrypointProc := exec.CommandContext(ctx, entrypointPath, "exec-plugin")

	// set pwd to plugin bin dir
	// entrypointProc.Dir = pluginDistDir

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(0)
	pluginStartInfo := &plugin.PluginStartInfo{
		InstanceId: pluginInstanceID,
	}
	// pluginStartInfoB58 := pluginStartInfo.MarshalB58()
	_ = pluginStartInfo

	// stderr: contains any logs
	// TODO: logging?
	// le := h.le.WithField("plugin-id", pluginID)
	// debugWriter := le.WriterLevel(logrus.DebugLevel)
	// entrypointProc.Stderr = debugWriter

	/* TODO
	le.
		WithField("entrypoint", entrypoint).
		Debugf("executing plugin entrypoint: %s", entrypointProc.String())
	startObj, err := startCmd(entrypointProc, preStartObj)
	if err != nil {
		return err
	}
	*/

	// wait for a non-nil error
	errCh := make(chan error, 3)
	exited := make(chan struct{})
	go func() {
		// errCh <- entrypointProc.Wait()
		close(exited)
	}()

	// fully kill & wait for exit to be confirmed when returning
	defer func() {
		ctxCancel()

		// TODO
		// _ = shutdownCmd(entrypointProc, preStartObj, startObj)

		// wait graceful shutdown max duration
		shutdownTimeout := time.NewTimer(time.Second * 3)
		select {
		case <-exited:
			_ = shutdownTimeout.Stop()
		case <-shutdownTimeout.C:
		}

		// _ = killCmd(entrypointProc, preStartObj, startObj)
		// TODO

		// wait for full shutdown
		<-exited
	}()

	// wait for context canceled and/or error
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *WebHost) DeletePlugin(ctx context.Context, pluginID string) error {
	// TODO remove caches or local storage?
	return nil
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WebHost)(nil)
