package core_all

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	egc "github.com/aperturerobotics/entitygraph/controller"
	"github.com/aperturerobotics/hydra/core"
	api_controller "github.com/aperturerobotics/hydra/daemon/api/controller"
	hydraeg "github.com/aperturerobotics/hydra/entitygraph"
	mysql_controller "github.com/aperturerobotics/hydra/sql/mysql/controller"
	unixfs_access_http "github.com/aperturerobotics/hydra/unixfs/access/http"
	unixfs_world_access "github.com/aperturerobotics/hydra/unixfs/world/access"
	volume_block "github.com/aperturerobotics/hydra/volume/block"
	volume_world "github.com/aperturerobotics/hydra/volume/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
)

// AddFactories adds all factories (including World Graph) to the static resolver.
// This is intended to keep the default Core as minimal as possible.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	core.AddFactories(b, sr)
	sr.AddFactory(api_controller.NewFactory(b))

	sr.AddFactory(world_block_engine.NewFactory(b))

	sr.AddFactory(volume_block.NewFactory(b))
	sr.AddFactory(volume_world.NewFactory(b))

	sr.AddFactory(unixfs_access_http.NewFactory(b))
	sr.AddFactory(unixfs_world_access.NewFactory(b))

	sr.AddFactory(mysql_controller.NewFactory(b))

	sr.AddFactory(egc.NewFactory(b))
	sr.AddFactory(hydraeg.NewFactory(b))
}
