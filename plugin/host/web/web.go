package plugin_host_web

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/aperturerobotics/bifrost/util/randstring"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
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
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
	// webRuntimeID is the identifier of the web runtime
	webRuntimeID string
}

// NewWebHost constructs a new WebHost.
func NewWebHost(b bus.Bus, le *logrus.Entry, webRuntimeID string) (*WebHost, error) {
	// determine the platform id for the host
	platformID := (&bldr_platform.WebPlatform{}).GetPlatformID()
	return &WebHost{
		b:                b,
		le:               le,
		pluginPlatformID: platformID,
		webRuntimeID:     webRuntimeID,
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
	processHost, err := NewWebHost(b, le, c.GetWebRuntimeId())
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
	h.le.Info("XXX Web ExecutePlugin Start")
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

	entrypointHttpPath := plugin.PluginDistHTTPPath(pluginID, entrypoint)

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(0)
	pluginStartInfo := &plugin.PluginStartInfo{
		InstanceId: pluginInstanceID,
	}

	// web worker create request
	pluginWebWorkerID := "bldr:plugin:" + pluginID
	pluginWebWorkerURL := pluginStartHttpPath(entrypointHttpPath, pluginStartInfo)
	pluginShared := true

	// stderr: contains any logs
	// TODO: logging?
	// le := h.le.WithField("plugin-id", pluginID)
	// debugWriter := le.WriterLevel(logrus.DebugLevel)
	// entrypointProc.Stderr = debugWriter

	h.le.
		WithField("entrypoint", entrypoint).
		WithField("web-runtime", h.webRuntimeID).
		Debugf("executing plugin entrypoint: %s via http at %s", entrypointFi.Name(), pluginWebWorkerURL)
	webRuntime, _, webRuntimeRef, err := web_runtime.ExLookupWebRuntime(ctx, h.b, false, h.webRuntimeID)
	if err != nil {
		return err
	}
	defer webRuntimeRef.Release()

	h.le.Info("XXX got webRuntime")
	docs, err := webRuntime.GetWebDocuments(ctx)
	if err != nil {
		return err
	}
	h.le.Infof("XXX got webRuntime with %d documents: %v", len(docs), docs)

	// Remove any old instances of the web worker.
	for _, doc := range docs {
		docWebWorkers, err := doc.GetWebWorkers(ctx)
		if err != nil {
			return err
		}
		for _, worker := range docWebWorkers {
			if worker.GetId() != pluginWebWorkerID {
				continue
			}
			h.le.
				WithFields(logrus.Fields{
					"web-document": doc.GetWebDocumentUuid(),
					"web-runtime":  h.webRuntimeID,
					"web-worker":   pluginWebWorkerID,
				}).
				Debug("removing old instance of web worker")
			_, err := worker.Remove(ctx)
			if err != nil {
				h.le.WithError(err).Warn("unable to remove old web worker instance")
			}
		}
	}

	// Create the new instance(s) of the web worker.
	var createdAny, createdShared bool
	var createErr error
	for _, doc := range docs {
		le := h.le.
			WithFields(logrus.Fields{
				"web-document": doc.GetWebDocumentUuid(),
				"web-runtime":  h.webRuntimeID,
				"web-worker":   pluginWebWorkerID,
			})
		le.Debug("creating web worker")
		createdWorker, err := doc.CreateWebWorker(ctx, pluginWebWorkerID, pluginShared, pluginWebWorkerURL)
		if createdWorker == nil && err == nil {
			err = errors.New("document did not create the worker")
		}
		if err != nil {
			le.WithError(err).Warn("unable to create web worker")
			// try to get the most meaningful error
			if createErr == nil || createErr == context.Canceled || createErr == io.EOF {
				createErr = err
			}
			continue
		}

		createdAny, createdShared = true, createdWorker.GetShared()
		le.WithField("shared-worker", createdShared).Debug("successfully created web worker")

		// If we cannot create shared workers, create only one Worker to avoid duplicates.
		// NOTE: This assumes that if shared=true works for one doc it will work for all on the same runtime.
		if !createdShared {
			break
		}
	}
	if !createdAny {
		h.le.WithError(createErr).Warn("unable to create any web workers")
		return createErr
	}

	/* TODO
	startObj, err := startCmd(entrypointProc, preStartObj)
	if err != nil {
		return err
	}
	*/

	// wait for a non-nil error
	errCh := make(chan error, 3)
	exited := make(chan struct{})
	go func() {
		// TODO
		// errCh <- entrypointProc.Wait()
		<-ctx.Done()
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

func pluginStartHttpPath(entrypointHttpPath string, pluginStartInfo *plugin.PluginStartInfo) string {
	var sb strings.Builder
	_, _ = sb.WriteString(plugin.PluginStartHttpPrefix)
	_, _ = sb.WriteString("plugin.mjs#")
	_, _ = sb.WriteString("e=")
	_, _ = sb.WriteString(entrypointHttpPath)
	_, _ = sb.WriteString("&si=")
	_, _ = sb.WriteString(pluginStartInfo.MarshalB58())
	return sb.String()
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WebHost)(nil)
