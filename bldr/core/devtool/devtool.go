package bldr_core_devtool

import (
	"github.com/s4wave/spacewave/net/transport/websocket"
	manifest_fetch_plugin "github.com/s4wave/spacewave/bldr/manifest/fetch/plugin"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
)

// addCommonFactories adds the factories common to all arches.
func addCommonFactories(b bus.Bus, sr *static.Resolver) {
	// hydra
	sr.AddFactory(world_block_engine.NewFactory(b))

	// transports
	sr.AddFactory(websocket.NewFactory(b))

	// plugin host
	for _, factory := range plugin_host_default.PluginHostControllerFactories {
		sr.AddFactory(factory(b))
	}

	// plugin scheduler
	sr.AddFactory(plugin_host_scheduler.NewFactory(b))

	// manifest fetch via plugin
	sr.AddFactory(manifest_fetch_plugin.NewFactory(b))

	// volume rpc server
	sr.AddFactory(volume_rpc_server.NewFactory(b))
}
