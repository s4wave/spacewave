package plugin_host

import (
	"context"
	"errors"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/starpc/srpc"
)

// ErrPluginUninitialized is returned if the plugin was not initialized.
var ErrPluginUninitialized = errors.New("plugin is not yet initialized")

// PluginRpcInitCb is a callback to be called when the RPC channel is ready.
type PluginRpcInitCb func(client srpc.Client) error

// PluginHost manages and executes plugins.
type PluginHost interface {
	// GetPlatformId returns the platform ID for this host.
	// The plugin host must be capable of executing plugin manifests with this platform id.
	// Must return a value.
	GetPlatformId() string

	// Execute executes the plugin host.
	// If an error is returned, the plugin host execution will be retried.
	// If nil is returned, this indicates PluginHost does not need the Execute goroutin.
	// Return context.Canceled if context was canceled.
	Execute(ctx context.Context) error

	// ListPlugins lists the set of loaded plugins in the host.
	ListPlugins(ctx context.Context) ([]string, error)

	// ExecutePlugin executes the plugin with the given ID.
	// If the plugin was already initialized, existing state can be reused.
	// The plugin should be stopped if/when the function exits.
	// Return ErrPluginUninitialized if the plugin was not ready.
	// Should expect to be called only once (at a time) for a plugin ID.
	// pluginDist contains the plugin distribution files (binaries and assets).
	// rpcInit is called when the RPC client is ready, should return a mux for the server.
	ExecutePlugin(
		ctx context.Context,
		pluginID,
		entrypoint string,
		pluginDist *unixfs.FSHandle,
		pluginAssets *unixfs.FSHandle,
		hostRpcMux srpc.Mux,
		rpcInit PluginRpcInitCb,
	) error

	// DeletePlugin clears cached plugin data for the given plugin ID.
	DeletePlugin(ctx context.Context, pluginID string) error
}

// PluginHostScheduler manages the PluginHosts and running plugins.
type PluginHostScheduler interface {
	// AddPluginReference adds a reference to the plugin, returning the RunningPlugin
	// handle and a release function.
	//
	// Returns nil, nil, err if any error occurs.
	AddPluginReference(pluginID string) (bldr_plugin.RunningPluginRef, func())
}
