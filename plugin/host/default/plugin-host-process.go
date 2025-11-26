//go:build !js && !wasip1

package plugin_host_default

import (
	"context"

	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	plugin_host_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
)

// PluginHostControllerFactories construct the plugin host controller factory.
var PluginHostControllerFactories = [](func(bus bus.Bus) controller.Factory){
	func(b bus.Bus) controller.Factory {
		return plugin_host_process.NewFactory(b)
	},
	func(b bus.Bus) controller.Factory {
		return plugin_host_quickjs.NewFactory(b)
	},
}

// PluginHostController is an alias to the plugin host controller type.
type PluginHostController struct {
	ProcessHost *plugin_host_controller.Controller
	QuickjsHost *plugin_host_controller.Controller
}

// StartPluginHost starts the plugin host on the controller bus.
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
	pluginHostProcessConf := plugin_host_process.NewConfig(pluginsStateRoot, pluginsDistRoot)
	processPluginHostCtrl, _, processPluginHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(pluginHostProcessConf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	pluginHostQuickjsConf := plugin_host_quickjs.NewConfig()
	quickjsHostCtrl, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(pluginHostQuickjsConf),
		nil,
	)
	if err != nil {
		processPluginHostRef.Release()
		return nil, nil, err
	}

	return &PluginHostController{
			ProcessHost: processPluginHostCtrl,
			QuickjsHost: quickjsHostCtrl,
		}, func() {
			quickjsHostRef.Release()
			processPluginHostRef.Release()
		}, nil
}
