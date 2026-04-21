package core_all

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	egc "github.com/aperturerobotics/entitygraph/controller"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_http "github.com/s4wave/spacewave/db/block/store/http"
	http_lookup "github.com/s4wave/spacewave/db/block/store/http/lookup"
	http_server "github.com/s4wave/spacewave/db/block/store/http/server"
	block_store_kvfile_http "github.com/s4wave/spacewave/db/block/store/kvfile/http"
	block_store_redis "github.com/s4wave/spacewave/db/block/store/redis"
	block_store_ristretto "github.com/s4wave/spacewave/db/block/store/ristretto"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_s3_lookup "github.com/s4wave/spacewave/db/block/store/s3/lookup"
	"github.com/s4wave/spacewave/db/core"
	api_controller "github.com/s4wave/spacewave/db/daemon/api/controller"
	hydraeg "github.com/s4wave/spacewave/db/entitygraph"
	mysql_controller "github.com/s4wave/spacewave/db/sql/mysql/controller"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
	unixfs_world_access "github.com/s4wave/spacewave/db/unixfs/world/access"
	volume_block "github.com/s4wave/spacewave/db/volume/block"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
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

	sr.AddFactory(http_lookup.NewFactory(b))
	sr.AddFactory(http_server.NewFactory(b))

	sr.AddFactory(block_store_bucket.NewFactory(b))
	sr.AddFactory(block_store_kvfile_http.NewFactory(b))
	sr.AddFactory(block_store_http.NewFactory(b))
	sr.AddFactory(block_store_s3.NewFactory(b))
	sr.AddFactory(block_store_s3_lookup.NewFactory(b))
	sr.AddFactory(block_store_ristretto.NewFactory(b))
	sr.AddFactory(block_store_redis.NewFactory(b))
}
