package core

import (
	"context"

	cresolve "github.com/aperturerobotics/bldr/assembly/bridge/cresolve"
	cvolume "github.com/aperturerobotics/bldr/assembly/bridge/volume"
	assembly_controller "github.com/aperturerobotics/bldr/assembly/controller"
	manifest_fetch_viaplugin "github.com/aperturerobotics/bldr/manifest/fetch/plugin"
	manifest_fetch_viaworld "github.com/aperturerobotics/bldr/manifest/fetch/world"
	handle_rpc_viaplugin "github.com/aperturerobotics/bldr/plugin/forward-rpc-service"
	handle_webview_viaplugin "github.com/aperturerobotics/bldr/plugin/handle-web-view"
	bldr_plugin_load "github.com/aperturerobotics/bldr/plugin/load"
	web_fetch_service "github.com/aperturerobotics/bldr/web/fetch/service"
	web_pkg_fs_controller "github.com/aperturerobotics/bldr/web/pkg/fs/controller"
	web_pkg_rpc_client "github.com/aperturerobotics/bldr/web/pkg/rpc/client"
	web_pkg_rpc_server "github.com/aperturerobotics/bldr/web/pkg/rpc/server"
	web_plugin_handle_rpc "github.com/aperturerobotics/bldr/web/plugin/handle-rpc"
	web_plugin_handle_web_pkg "github.com/aperturerobotics/bldr/web/plugin/handle-web-pkg"
	web_plugin_handle_web_view "github.com/aperturerobotics/bldr/web/plugin/handle-web-view"
	web_view_handler_via_bus "github.com/aperturerobotics/bldr/web/view/handler/server"
	web_view_observer "github.com/aperturerobotics/bldr/web/view/observer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	block_store_http "github.com/aperturerobotics/hydra/block/store/http"
	http_lookup "github.com/aperturerobotics/hydra/block/store/http/lookup"
	http_server "github.com/aperturerobotics/hydra/block/store/http/server"
	block_store_ristretto "github.com/aperturerobotics/hydra/block/store/ristretto"
	block_store_rpc "github.com/aperturerobotics/hydra/block/store/rpc"
	block_store_rpc_lookup "github.com/aperturerobotics/hydra/block/store/rpc/lookup"
	block_store_rpc_server "github.com/aperturerobotics/hydra/block/store/rpc/server"
	block_store_s3 "github.com/aperturerobotics/hydra/block/store/s3"
	block_store_s3_lookup "github.com/aperturerobotics/hydra/block/store/s3/lookup"
	hydracore "github.com/aperturerobotics/hydra/core"
	unixfs_world_access "github.com/aperturerobotics/hydra/unixfs/world/access"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
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
func AddFactories(b bus.Bus, sr *static.Resolver) {
	hydracore.AddFactories(b, sr)

	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(unixfs_world_access.NewFactory(b))
	sr.AddFactory(assembly_controller.NewFactory(b))

	sr.AddFactory(cresolve.NewFactory(b))
	sr.AddFactory(cvolume.NewFactory(b))

	sr.AddFactory(manifest_fetch_viaplugin.NewFactory(b))
	sr.AddFactory(manifest_fetch_viaworld.NewFactory(b))

	sr.AddFactory(bldr_plugin_load.NewFactory(b))
	sr.AddFactory(handle_webview_viaplugin.NewFactory(b))
	sr.AddFactory(handle_rpc_viaplugin.NewFactory(b))

	sr.AddFactory(web_view_observer.NewFactory(b))
	sr.AddFactory(web_fetch_service.NewFactory(b))

	sr.AddFactory(web_plugin_handle_rpc.NewFactory(b))
	sr.AddFactory(web_plugin_handle_web_pkg.NewFactory(b))
	sr.AddFactory(web_plugin_handle_web_view.NewFactory(b))

	sr.AddFactory(web_pkg_fs_controller.NewFactory(b))
	sr.AddFactory(web_pkg_rpc_client.NewFactory(b))
	sr.AddFactory(web_pkg_rpc_server.NewFactory(b))
	sr.AddFactory(web_view_handler_via_bus.NewFactory(b))

	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(volume_rpc_client.NewFactory(b))

	sr.AddFactory(http_lookup.NewFactory(b))
	sr.AddFactory(http_server.NewFactory(b))

	sr.AddFactory(block_store_http.NewFactory(b))
	sr.AddFactory(block_store_s3.NewFactory(b))
	sr.AddFactory(block_store_s3_lookup.NewFactory(b))
	sr.AddFactory(block_store_ristretto.NewFactory(b))

	sr.AddFactory(block_store_rpc.NewFactory(b))
	sr.AddFactory(block_store_rpc_lookup.NewFactory(b))
	sr.AddFactory(block_store_rpc_server.NewFactory(b))
}
