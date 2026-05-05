package bldr_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
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
// Component ID: remote plugin id
// Invokes the rpc on the bus with the server id set to plugin/remote plugin id.
func (s *PluginServer) PluginRpc(rpcStream SRPCPlugin_PluginRpcStream) error {
	return rpcstream.HandleRpcStream(
		rpcStream,
		func(
			ctx context.Context,
			remotePluginID string,
			released func(),
		) (srpc.Invoker, func(), error) {
			if remotePluginID == "" {
				return nil, nil, errors.Wrap(ErrEmptyPluginID, "remote plugin rpc")
			}
			baseRemoteServerID := PluginServerID(remotePluginID, "")
			invoker := bifrost_rpc.NewInvoker(s.b, baseRemoteServerID, true)
			rootMux := srpc.NewMux(invoker)
			mux := srpc.NewMux(invoker)
			accessRpcServiceServer := bifrost_rpc_access.NewAccessRpcServiceServer(
				s.b,
				true,
				func(remoteServerID string) (string, error) {
					return PluginServerID(remotePluginID, remoteServerID), nil
				},
			)
			_ = bifrost_rpc_access.SRPCRegisterAccessRpcService(mux, accessRpcServiceServer)
			resourceServer := resource_server.NewResourceServer(rootMux)
			_ = resourceServer.Register(mux)
			return mux, nil, nil
		},
	)
}

// _ is a type assertion
var _ SRPCPluginServer = ((*PluginServer)(nil))
