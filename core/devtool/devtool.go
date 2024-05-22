package bldr_core_devtool

import (
	"github.com/aperturerobotics/bifrost/transport/websocket"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
)

// addCommonFactories adds the factories common to all arches.
func addCommonFactories(b bus.Bus, sr *static.Resolver) {
	// hydra
	sr.AddFactory(world_block_engine.NewFactory(b))

	// transports
	sr.AddFactory(websocket.NewFactory(b))

	// plugin host
	sr.AddFactory(plugin_host_default.NewPluginHostControllerFactory(b))
}
