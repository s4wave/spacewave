package bldr_core_devtool

import (
	"github.com/aperturerobotics/bldr/core"
	bldr_plugin_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	plugin_host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	bldr_project_watcher "github.com/aperturerobotics/bldr/project/watcher"
	plugin_web "github.com/aperturerobotics/bldr/web/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
)

// AddFactories adds the devtool factories.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	core.AddFactories(b, sr)

	// add controller factories
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(plugin_host_process.NewFactory(b))
	sr.AddFactory(bldr_project_watcher.NewFactory(b))
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(bldr_plugin_builder_controller.NewFactory(b))
	sr.AddFactory(plugin_compiler.NewFactory(b))
	sr.AddFactory(plugin_web.NewFactory(b))
}
