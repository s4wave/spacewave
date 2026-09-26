// Package plugin_host_mock provides plugin host test doubles.
package plugin_host_mock

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/unixfs"
)

// Host is a plugin host that reports a platform ID and runs nothing.
type Host struct {
	// platformID is the reported platform ID.
	platformID string
}

// NewHost constructs a host for platformID.
func NewHost(platformID string) *Host {
	return &Host{platformID: platformID}
}

// GetPlatformId returns the platform ID.
func (h *Host) GetPlatformId() string {
	return h.platformID
}

// Execute returns immediately.
func (h *Host) Execute(context.Context) error {
	return nil
}

// ListPlugins returns no plugins.
func (h *Host) ListPlugins(context.Context) ([]string, error) {
	return nil, nil
}

// ExecutePlugin returns immediately.
func (h *Host) ExecutePlugin(
	context.Context,
	string,
	string,
	string,
	string,
	*unixfs.FSHandle,
	*unixfs.FSHandle,
	srpc.Mux,
	plugin_host.PluginRpcInitCb,
) error {
	return nil
}

// DeletePlugin returns immediately.
func (h *Host) DeletePlugin(context.Context, string) error {
	return nil
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*Host)(nil)
