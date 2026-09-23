package resource_worldop_registry

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// applicationCore exposes the native core Resource tree at the plugin boundary.
// The application worker still crosses the real plugin and Resource transports.
type applicationCore struct {
	root   srpc.Invoker
	client srpc.Client
}

// newApplicationCore constructs the Resource-serving core plugin capability.
func newApplicationCore(root srpc.Invoker) *applicationCore {
	core := &applicationCore{root: root}
	mux := srpc.NewMux()
	_ = plugin.SRPCRegisterPlugin(mux, core)
	core.client = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	return core
}

// GetControllerInfo identifies this test core provider.
func (c *applicationCore) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/application-core", controller.MustParseVersion("0.0.1"), "native core Resource capability")
}

// Execute keeps the core provider available for the test lifetime.
func (c *applicationCore) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// HandleDirective serves only the core family; the real scheduler runs the app.
func (c *applicationCore) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "spacewave-core" {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]plugin.LoadPluginValue{plugin.NewRunningPlugin(c.client)}), nil)
}

// Close releases no state beyond the test's bus lifetime.
func (c *applicationCore) Close() error {
	return nil
}

// PluginRpc forwards application calls to the real native Resource server.
func (c *applicationCore) PluginRpc(strm plugin.SRPCPlugin_PluginRpcStream) error {
	return rpcstream.HandleRpcStream(strm, func(context.Context, string, func()) (srpc.Invoker, func(), error) {
		return c.root, nil, nil
	})
}

var _ controller.Controller = (*applicationCore)(nil)
