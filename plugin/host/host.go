package plugin_host

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/unixfs"
)

// ErrPluginUninitialized is returned if the plugin was not initialized.
var ErrPluginUninitialized = errors.New("plugin is not yet initialized")

// PluginHost manages and executes plugins.
type PluginHost interface {
	// ListPlugins lists the set of initialized plugins.
	ListPlugins(ctx context.Context) ([]string, error)
	// ExecutePlugin executes the plugin with the given ID.
	// If the plugin was already initialized, existing state can be reused.
	// The plugin should be stopped if/when the function exits.
	// Return ErrPluginUninitialized if the plugin was not ready.
	// Should expect to be called only once (at a time) for a plugin ID.
	// pluginDist contains the plugin distribution files (binaries and assets).
	ExecutePlugin(ctx context.Context, pluginID string, pluginDist *unixfs.FSHandle) error
	// DeletePlugin clears cached plugin data for the given plugin ID.
	DeletePlugin(ctx context.Context, pluginID string) error
}
