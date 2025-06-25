package dist_entrypoint

import (
	"context"

	manifest_fetch_viaplugin "github.com/aperturerobotics/bldr/manifest/fetch/plugin"
	manifest_fetch_viaworld "github.com/aperturerobotics/bldr/manifest/fetch/world"
	handle_rpc_viaplugin "github.com/aperturerobotics/bldr/plugin/forward-rpc-service"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/aperturerobotics/bldr/plugin/host/scheduler"
	bldr_plugin_load "github.com/aperturerobotics/bldr/plugin/load"
	storage_default "github.com/aperturerobotics/bldr/storage/default"
	storage_volume "github.com/aperturerobotics/bldr/storage/volume"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	block_store_bucket "github.com/aperturerobotics/hydra/block/store/bucket"
	block_store_rpc "github.com/aperturerobotics/hydra/block/store/rpc"
	block_store_rpc_lookup "github.com/aperturerobotics/hydra/block/store/rpc/lookup"
	block_store_rpc_server "github.com/aperturerobotics/hydra/block/store/rpc/server"
	block_store_s3_lookup "github.com/aperturerobotics/hydra/block/store/s3/lookup"
	lookup_concurrent "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	unixfs_world_access "github.com/aperturerobotics/hydra/unixfs/world/access"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a bus for the dist entrypoint.
func NewCoreBus(
	ctx context.Context,
	le *logrus.Entry,
	opts ...cbc.Option,
) (bus.Bus, *static.Resolver, error) {
	b, sr, err := cbc.NewCoreBus(ctx, le, opts...)
	if err != nil {
		return nil, nil, err
	}

	AddFactories(b, sr)
	return b, sr, nil
}

// AddFactories adds factories to an existing static resolver.
// NOTE: Only add a factory here if it is absolutely needed by the entrypoint.
// NOTE: this list will differ depending on the platform.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(bldr_plugin_load.NewFactory(b))
	sr.AddFactory(handle_rpc_viaplugin.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(plugin_host_scheduler.NewFactory(b))
	sr.AddFactory(plugin_host_default.NewPluginHostControllerFactory(b))
	sr.AddFactory(unixfs_world_access.NewFactory(b))
	sr.AddFactory(volume_rpc_client.NewFactory(b))
	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(manifest_fetch_viaplugin.NewFactory(b))
	sr.AddFactory(manifest_fetch_viaworld.NewFactory(b))
	sr.AddFactory(block_store_bucket.NewFactory(b))
	sr.AddFactory(block_store_rpc.NewFactory(b))
	sr.AddFactory(block_store_rpc_lookup.NewFactory(b))
	sr.AddFactory(block_store_rpc_server.NewFactory(b))
	sr.AddFactory(block_store_s3_lookup.NewFactory(b))
	sr.AddFactory(storage_volume.NewFactory(b))
	for _, st := range storage_default.BuildStorage(b, "") {
		st.AddFactories(b, sr)
	}
}
