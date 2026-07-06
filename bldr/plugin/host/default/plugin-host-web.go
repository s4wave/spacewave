//go:build js || wasip1 || wasm

package plugin_host_default

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	plugin_host_web "github.com/s4wave/spacewave/bldr/plugin/host/web"
)

// PluginHostControllerFactories construct the plugin host controller factory.
var PluginHostControllerFactories = [](func(bus bus.Bus) controller.Factory){
	func(b bus.Bus) controller.Factory {
		return plugin_host_web.NewFactory(b)
	},
}

// PluginHostController contains the plugin host controllers.
type PluginHostController struct {
	WebHost *plugin_host_controller.Controller
	JsHost  *plugin_host_controller.Controller
}

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
	webHostConf := plugin_host_web.NewConfig(webRuntimeID, "web/js/wasm")
	webHostCtrl, _, webHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(webHostConf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	jsHostConf := plugin_host_web.NewConfig(webRuntimeID, bldr_platform.PlatformID_JS)
	jsHostCtrl, _, jsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(jsHostConf),
		nil,
	)
	if err != nil {
		webHostRef.Release()
		return nil, nil, err
	}

	return &PluginHostController{
			WebHost: webHostCtrl,
			JsHost:  jsHostCtrl,
		}, func() {
			jsHostRef.Release()
			webHostRef.Release()
		}, nil
}
