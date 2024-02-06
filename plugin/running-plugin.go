package bldr_plugin

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
)

// RunningPlugin is the interface exposed to callers of LoadPlugin.
type RunningPlugin interface {
	// GetRpcClient returns the RPC client.
	GetRpcClient() srpc.Client
}

// RunningPluginRef is a reference to a running plugin.
type RunningPluginRef interface {
	// GetRunningPluginCtr returns the current running plugin instance.
	// May be changed (or set to nil) when the instance changes.
	GetRunningPluginCtr() ccontainer.Watchable[RunningPlugin]
}

// runningPlugin contains a static srpc client
type runningPlugin struct {
	rpcClient srpc.Client
}

// NewRunningPlugin constructs a RunningPlugin with a static client.
func NewRunningPlugin(client srpc.Client) RunningPlugin {
	return &runningPlugin{rpcClient: client}
}

// GetRpcClient returns the RPC client.
func (r *runningPlugin) GetRpcClient() srpc.Client {
	return r.rpcClient
}

// _ is a type assertion
var _ RunningPlugin = ((*runningPlugin)(nil))
