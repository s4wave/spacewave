package dist_entrypoint

import (
	"context"

	manifest_fetch_viaplugin "github.com/aperturerobotics/bldr/manifest/fetch/via-plugin"
	handle_rpc_viaplugin "github.com/aperturerobotics/bldr/plugin/forward-rpc-service"
	plugin_host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
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
	sr.AddFactory(handle_rpc_viaplugin.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(plugin_host_process.NewFactory(b))
	sr.AddFactory(unixfs_world_access.NewFactory(b))
	sr.AddFactory(volume_rpc_client.NewFactory(b))
	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(manifest_fetch_viaplugin.NewFactory(b))

	// sr.AddFactory(handle_webview_viaplugin.NewFactory(b))
	// sr.AddFactory(plugin_compiler.NewFactory(b))
}
