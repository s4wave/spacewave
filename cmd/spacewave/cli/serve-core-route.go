//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// registerNativeCoreResource exposes the existing native Resource server under
// the route plugin hosts use to reach core. Its lifetime is the serving daemon;
// Dist mode continues to route to the deployed core plugin instead.
func registerNativeCoreResource(ctx context.Context, b bus.Bus, invoker srpc.Invoker) (func(), error) {
	serviceID := bldr_plugin.PluginServiceID("spacewave-core", resource.SRPCResourceServiceServiceID)
	// The qualified name selects core; Resource clients then use the ordinary
	// service name on the returned connection. Accept either at this route.
	route := srpc.InvokerFunc(func(service, method string, stream srpc.Stream) (bool, error) {
		if service == serviceID {
			service = resource.SRPCResourceServiceServiceID
		}
		return invoker.InvokeMethod(service, method, stream)
	})
	ctrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("cli/native-core-resource", controller.MustParseVersion("0.0.1"), "native core Resource route"),
		bifrost_rpc.NewRpcServiceBuilder(route),
		nil,
		false,
		nil,
		[]string{serviceID},
		nil,
	)
	return b.AddController(ctx, ctrl, nil)
}
