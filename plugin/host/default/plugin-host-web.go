//go:build js || wasip1 || wasm

package plugin_host_default

import (
	"context"

	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
)

// PluginHostController is an alias to the plugin host controller type.
type PluginHostController = plugin_host_controller.Controller

// StartBusPluginHost starts the plugin host.
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
	pluginHostProcessConf := plugin_host_web.NewConfig(
		plugin_host_controller.NewConfig(
			engineID,
			pluginHostObjectKey,
			volID,
			volPeerID,
			alwaysFetchManifest,
			disableStoreManifest,
		),
		webRuntimeID,
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
var NewPluginHostControllerFactory = plugin_host_web.NewFactory
