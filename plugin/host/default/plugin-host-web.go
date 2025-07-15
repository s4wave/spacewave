//go:build js || wasip1 || wasm

package plugin_host_default

import (
	"context"

	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
)

// PluginHostControllerFactories construct the plugin host controller factory.
var PluginHostControllerFactories = [](func(bus bus.Bus) controller.Factory){
	func(b bus.Bus) controller.Factory {
		return plugin_host_web.NewFactory(b)
	},
}

// PluginHostController is an alias to the plugin host controller type.
type PluginHostController = plugin_host_controller.Controller

// StartPluginHost starts the plugin host.
//
// webRuntimeID is ignored on the native platform as the web runtime is bundled into the web plugin.
// pluginsStateRoot and pluginsDistRoot are ignored on the web platform as IndexedDB is used.
func StartPluginHost(
	ctx context.Context,
	b bus.Bus,
	pluginsStateRoot,
	pluginsDistRoot string,
	webRuntimeID string,
) (ctrl *PluginHostController, rel func(), err error) {
	pluginHostProcessConf := plugin_host_web.NewConfig(webRuntimeID)
	pluginHostCtrl, _, pluginHostRef, err := loader.WaitExecControllerRunningTyped[*PluginHostController](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(pluginHostProcessConf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return pluginHostCtrl, pluginHostRef.Release, nil
}
