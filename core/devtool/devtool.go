package bldr_core_devtool

import (
	"github.com/aperturerobotics/bifrost/transport/websocket"
	dist_compiler "github.com/aperturerobotics/bldr/dist/compiler"
	bldr_plugin_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	bldr_project_watcher "github.com/aperturerobotics/bldr/project/watcher"
	web_pkg_compiler "github.com/aperturerobotics/bldr/web/pkg/compiler"
	web_plugin_compiler "github.com/aperturerobotics/bldr/web/plugin/compiler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
)

// AddFactories adds the devtool factories.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	// volumes
	sr.AddFactory(volume_bolt.NewFactory(b))

	// transports
	sr.AddFactory(websocket.NewFactory(b))

	// project
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(bldr_project_watcher.NewFactory(b))

	// compiler
	sr.AddFactory(bldr_plugin_builder_controller.NewFactory(b))
	sr.AddFactory(dist_compiler.NewFactory(b))
	sr.AddFactory(plugin_compiler.NewFactory(b))
	sr.AddFactory(web_pkg_compiler.NewFactory(b))
	sr.AddFactory(web_plugin_compiler.NewFactory(b))

	// plugin host
	sr.AddFactory(plugin_host_default.NewPluginHostControllerFactory(b))
}
