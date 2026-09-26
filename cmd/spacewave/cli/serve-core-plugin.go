//go:build !js

package spacewave_cli

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// nativeCorePluginID is the plugin native core stands in for.
const nativeCorePluginID = "spacewave-core"

// nativeCorePlugin presents the native core Resource server as the
// spacewave-core plugin, so plugins reach core the same way natively and in
// Dist mode. It resolves loads of the current core, from any plugin instance,
// and plugin/spacewave-core/ service lookups. Its lifetime is the serving
// daemon.
type nativeCorePlugin struct {
	// invoker serves the native core services.
	invoker srpc.Invoker
	// client reaches the core services and the Plugin service.
	client srpc.Client
}

// newNativeCorePlugin constructs the core plugin over the native invoker.
func newNativeCorePlugin(invoker srpc.Invoker) *nativeCorePlugin {
	p := &nativeCorePlugin{invoker: invoker}
	mux := srpc.NewMux(invoker)
	_ = bldr_plugin.SRPCRegisterPlugin(mux, p)
	p.client = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	return p
}

// GetControllerInfo returns information about the controller.
func (p *nativeCorePlugin) GetControllerInfo() *controller.Info {
	return controller.NewInfo("cli/native-core-plugin", controller.MustParseVersion("0.0.1"), "native core as the spacewave-core plugin")
}

// Execute keeps the core plugin available until the daemon stops.
func (p *nativeCorePlugin) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// HandleDirective resolves core plugin loads and service lookups.
func (p *nativeCorePlugin) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bldr_plugin.LoadPlugin:
		// A load that selects exact manifests names an executable native core
		// does not have.
		if d.LoadPluginID() != nativeCorePluginID || d.LoadPluginManifestRoot() != "" || len(d.LoadPluginManifests()) != 0 {
			return nil, nil
		}
		return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(p.client)}), nil)
	case bifrost_rpc.LookupRpcService:
		if !strings.HasPrefix(d.LookupRpcServiceID(), bldr_plugin.PluginServiceID(nativeCorePluginID, "")) {
			return nil, nil
		}
		return directive.R(bldr_plugin.ResolveLookupRpcService(ctx, d, p))
	}
	return nil, nil
}

// WaitPluginHostClient declines: native core is not a plugin host.
func (p *nativeCorePlugin) WaitPluginHostClient(context.Context, func()) (srpc.Client, func(), error) {
	return nil, nil, nil
}

// WaitPluginClient returns the core client, which lives as long as p.
func (p *nativeCorePlugin) WaitPluginClient(context.Context, func(), string) (srpc.Client, func(), error) {
	return p.client, nil, nil
}

// PluginRpc serves another plugin's calls to core with the native services.
func (p *nativeCorePlugin) PluginRpc(strm bldr_plugin.SRPCPlugin_PluginRpcStream) error {
	return rpcstream.HandleRpcStream(strm, func(context.Context, string, func()) (srpc.Invoker, func(), error) {
		return p.invoker, nil, nil
	})
}

// Close releases no state beyond the daemon bus lifetime.
func (p *nativeCorePlugin) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller              = (*nativeCorePlugin)(nil)
	_ bldr_plugin.LookupRpcClientHandler = (*nativeCorePlugin)(nil)
	_ bldr_plugin.SRPCPluginServer       = (*nativeCorePlugin)(nil)
)
