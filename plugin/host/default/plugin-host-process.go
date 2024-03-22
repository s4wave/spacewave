//go:build !js && !wasip1

package plugin_host_default

import (
	"context"

	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	plugin_host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
)

// PluginHostController is an alias to the plugin host controller type.
type PluginHostController = plugin_host_controller.Controller

// StartBusPluginHost starts the plugin host.
//
// webRuntimeID is ignored on the native platform as the web runtime is bundled into the web plugin.
func StartBusPluginHost(
	ctx context.Context,
	b bus.Bus,
	engineID,
	pluginHostObjectKey,
	volID,
	volPeerID,
	pluginsStateRoot,
	pluginsDistRoot string,
	alwaysFetchManifest,
	disableStoreManifest bool,
	webRuntimeID string,
) (ctrl *PluginHostController, rel func(), err error) {
	pluginHostProcessConf := host_process.NewConfig(
		plugin_host_controller.NewConfig(
			engineID,
			pluginHostObjectKey,
			volID,
			volPeerID,
			alwaysFetchManifest,
			disableStoreManifest,
		),
		pluginsStateRoot,
		pluginsDistRoot,
	)
	pluginHostCtrlObj, _, pluginHostRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(pluginHostProcessConf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	pluginHostCtrl := pluginHostCtrlObj.(*PluginHostController)
	return pluginHostCtrl, pluginHostRef.Release, nil
}

// NewPluginHostControllerFactory constructs the plugin host controller factory.
var NewPluginHostControllerFactory = plugin_host_process.NewFactory
