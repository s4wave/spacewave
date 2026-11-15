package bldr_plugin

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	bifrost_rpc_access "github.com/s4wave/spacewave/net/rpc/access"
)

// PluginServer implements the plugin rpc server.
type PluginServer struct {
	// b is the bus to invoke rpc calls
	b bus.Bus
}

// NewPluginServer constructs the plugin rpc server.
func NewPluginServer(b bus.Bus) *PluginServer {
	return &PluginServer{b: b}
}

// PluginRpc handles an RPC call from a remote plugin.
// Component ID: the caller's PluginCaller, encoded by the host.
// Invokes the rpc on the bus with the server id set to plugin/<caller id>.
func (s *PluginServer) PluginRpc(rpcStream SRPCPlugin_PluginRpcStream) error {
	return ServePluginRpc(rpcStream, func(caller *PluginCaller) srpc.Invoker {
		// Route the caller's calls to its own plugin/<id> services on the bus.
		remotePluginID := caller.GetPluginId()
		baseRemoteServerID := PluginServerID(remotePluginID, "")
		invoker := bifrost_rpc.NewInvoker(s.b, baseRemoteServerID, true)
		mux := srpc.NewMux(invoker)
		accessRpcServiceServer := bifrost_rpc_access.NewAccessRpcServiceServer(
			s.b,
			true,
			func(remoteServerID string) (string, error) {
				return PluginServerID(remotePluginID, remoteServerID), nil
			},
		)
		_ = bifrost_rpc_access.SRPCRegisterAccessRpcService(mux, accessRpcServiceServer)
		return mux
	})
}

// _ is a type assertion
var _ SRPCPluginServer = (*PluginServer)(nil)
