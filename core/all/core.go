package core_all

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	bucket_setup "github.com/aperturerobotics/hydra/bucket/setup"
	"github.com/aperturerobotics/hydra/core"
	api_controller "github.com/aperturerobotics/hydra/daemon/api/controller"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
)

// AddFactories adds all factories (including World Graph) to the static resolver.
// This is intended to keep the default Core as minimal as possible.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	core.AddFactories(b, sr)
	sr.AddFactory(api_controller.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(bucket_setup.NewFactory(b))
}
