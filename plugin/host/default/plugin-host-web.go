//go:build js || wasip1 || wasm

package plugin_host_default

import (
	"context"

	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	plugin_host_web_wasivm "github.com/aperturerobotics/bldr/plugin/host/web-wasivm"
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
	func(b bus.Bus) controller.Factory {
		return plugin_host_web.NewQuickJSFactory(b)
	},
	func(b bus.Bus) controller.Factory {
		return plugin_host_web_wasivm.NewFactory(b)
	},
}

// PluginHostController contains the plugin host controllers.
type PluginHostController struct {
	WebHost     *plugin_host_controller.Controller
	QuickJSHost *plugin_host_controller.Controller
	WasiVMHost  *plugin_host_controller.Controller
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
	webHostConf := plugin_host_web.NewConfig(webRuntimeID)
	webHostCtrl, _, webHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(webHostConf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	quickjsHostConf := plugin_host_web.NewQuickJSConfig(webRuntimeID)
	quickjsHostCtrl, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(quickjsHostConf),
		nil,
	)
	if err != nil {
		webHostRef.Release()
		return nil, nil, err
	}

	wasivmHostConf := &plugin_host_web_wasivm.Config{}
	wasivmHostCtrl, _, wasivmHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(wasivmHostConf),
		nil,
	)
	if err != nil {
		quickjsHostRef.Release()
		webHostRef.Release()
		return nil, nil, err
	}

	return &PluginHostController{
			WebHost:     webHostCtrl,
			QuickJSHost: quickjsHostCtrl,
			WasiVMHost:  wasivmHostCtrl,
		}, func() {
			wasivmHostRef.Release()
			quickjsHostRef.Release()
			webHostRef.Release()
		}, nil
}
