package plugin_host

import "github.com/aperturerobotics/starpc/srpc"

// PluginStateSnapshot contains a snapshot of status info / handles for a plugin.
type PluginStateSnapshot struct {
	// PluginId is the plugin identifier.
	PluginId string
	// RpcClient is the plugin RPC client.
	// Can be nil until the client is ready.
	RpcClient srpc.Client
}

// NewPluginState constructs a plugin state.
func NewPluginStateSnapshot(pluginID string, rpcClient srpc.Client) *PluginStateSnapshot {
	return &PluginStateSnapshot{
		PluginId:  pluginID,
		RpcClient: rpcClient,
	}
}
