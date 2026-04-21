package core

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	handle_rpc_viaplugin "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	handle_webview_viaplugin "github.com/s4wave/spacewave/bldr/plugin/handle-web-view"
	bldr_plugin_load "github.com/s4wave/spacewave/bldr/plugin/load"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_rpc_lookup "github.com/s4wave/spacewave/db/block/store/rpc/lookup"
	block_store_rpc_server "github.com/s4wave/spacewave/db/block/store/rpc/server"
	block_store_rpc_server_bucket "github.com/s4wave/spacewave/db/block/store/rpc/server/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	stream_srpc_server_lookup "github.com/s4wave/spacewave/net/stream/srpc/server/lookup"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus with the controllers.
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
//
// NOTE: We only add the essential factories here to keep binary sizes low.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(bucket_setup.NewFactory(b))

	sr.AddFactory(bldr_plugin_load.NewFactory(b))
	sr.AddFactory(handle_webview_viaplugin.NewFactory(b))
	sr.AddFactory(handle_rpc_viaplugin.NewFactory(b))

	sr.AddFactory(storage_volume.NewFactory(b))

	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
	sr.AddFactory(volume_rpc_client.NewFactory(b))

	sr.AddFactory(block_store_bucket.NewFactory(b))
	sr.AddFactory(block_store_rpc.NewFactory(b))
	sr.AddFactory(block_store_rpc_lookup.NewFactory(b))
	sr.AddFactory(block_store_rpc_server.NewFactory(b))
	sr.AddFactory(block_store_rpc_server_bucket.NewFactory(b))

	sr.AddFactory(stream_srpc_server_lookup.NewFactory(b))
}
