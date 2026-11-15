package bldr_web_plugin_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_forward_rpc_service "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// RpcForwarder forwards the calls of this plugin's web runtime to the plugin
// that asked for them with HandleRpcViaPlugin.
//
// Invariant: it never forwards a call that arrived from outside this plugin,
// whatever server ID pattern the target plugin sent. Such a call carries a
// server ID one of this plugin's muxes assigned (bldr_plugin.IsIncomingServerID),
// and forwarding it would let the caller reach the target under this plugin's
// name. A lookup the pattern matches under such an ID fails at once with
// bldr_plugin.ErrRelayRefused.
type RpcForwarder struct {
	*plugin_forward_rpc_service.Controller
}

// NewRpcForwarder constructs the forwarder for conf.
func NewRpcForwarder(le *logrus.Entry, b bus.Bus, conf *plugin_forward_rpc_service.Config) *RpcForwarder {
	return &RpcForwarder{Controller: plugin_forward_rpc_service.NewController(le, b, conf)}
}

// HandleDirective forwards the lookups the target's pattern matches and
// refuses those that arrived from outside this plugin.
func (f *RpcForwarder) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	res, err := f.Controller.HandleDirective(ctx, di)
	dir, ok := di.GetDirective().(bifrost_rpc.LookupRpcService)
	if ok && len(res) != 0 && bldr_plugin.IsIncomingServerID(dir.LookupRpcServerID()) {
		return directive.R(bldr_plugin.NewRelayRefusedResolver(dir.LookupRpcServerID()), nil)
	}
	return res, err
}

// _ is a type assertion
var _ controller.Controller = (*RpcForwarder)(nil)
