//go:build js || wasip1

package plugin_host_web_wasivm

import (
	"context"
	"path/filepath"
	"strings"

	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the web-wasivm plugin host controller ID.
const ControllerID = "bldr/plugin/host/web-wasivm"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// WebWasiVMHost implements the plugin host with WASI VM processes in the browser.
type WebWasiVMHost struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// conf is the configuration
	conf *Config
}

// NewWebWasiVMHost constructs a new WebWasiVMHost.
func NewWebWasiVMHost(b bus.Bus, le *logrus.Entry, conf *Config) *WebWasiVMHost {
	return &WebWasiVMHost{
		b:    b,
		le:   le,
		conf: conf,
	}
}

// NewWebWasiVMHostController constructs the WebWasiVMHost and PluginHost controller.
func NewWebWasiVMHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *Config,
) (*host_controller.Controller, *WebWasiVMHost, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	pluginHost := NewWebWasiVMHost(b, le, c)
	hctrl := host_controller.NewController(
		le,
		b,
		controller.NewInfo(ControllerID, Version, "plugin host with WASI VM in browser"),
		pluginHost,
	)
	return hctrl, pluginHost, nil
}

// GetPlatformId returns the plugin platform ID for this host.
func (h *WebWasiVMHost) GetPlatformId() string {
	return "web/wasi/wasm"
}

// Execute returns nil as the web-wasivm host does not need a background goroutine.
func (h *WebWasiVMHost) Execute(ctx context.Context) error {
	return nil
}

// ListPlugins lists the set of initialized plugins.
func (h *WebWasiVMHost) ListPlugins(ctx context.Context) ([]string, error) {
	return nil, nil
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *WebWasiVMHost) DeletePlugin(ctx context.Context, pluginID string) error {
	return nil
}

// ExecutePlugin executes the plugin with the given ID.
func (h *WebWasiVMHost) ExecutePlugin(
	ctx context.Context,
	pluginID, instanceKey, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	// restrict to .wasm only
	if !strings.HasSuffix(entrypoint, ".wasm") {
		return errors.Errorf("entrypoint must have a .wasm extension: %q", entrypoint)
	}

	// check the entrypoint exists and is a regular file
	entrypoint = filepath.Clean(entrypoint)
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrapf(err, "entrypoint at %s", entrypoint)
	}

	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}

	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be a regular file: %s", entrypointFiMode.String())
	}

	h.le.
		WithField("entrypoint", entrypoint).
		WithField("plugin-id", pluginID).
		Debug("web-wasivm host starting plugin execution")

	// TODO: Read the WASM binary from pluginDist.
	// TODO: Create BrowserRuntime, boot kernel, bridge SRPC.

	<-ctx.Done()
	return context.Canceled
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WebWasiVMHost)(nil)
