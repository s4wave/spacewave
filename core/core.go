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
	web_plugin_handle_rpc "github.com/aperturerobotics/bldr/web/plugin/handle-rpc"
	web_plugin_handle_web_view "github.com/aperturerobotics/bldr/web/plugin/handle-web-view"
	web_view_observer "github.com/aperturerobotics/bldr/web/view/observer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	http_lookup "github.com/aperturerobotics/hydra/block/store/http/lookup"
	hydracore "github.com/aperturerobotics/hydra/core"
	unixfs_world_access "github.com/aperturerobotics/hydra/unixfs/world/access"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
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

	sr.AddFactory(assembly_controller.NewFactory(b))
	sr.AddFactory(cresolve.NewFactory(b))
	sr.AddFactory(cvolume.NewFactory(b))
	sr.AddFactory(bldr_plugin_load.NewFactory(b))
	sr.AddFactory(manifest_fetch_viaplugin.NewFactory(b))
	sr.AddFactory(manifest_fetch_viaworld.NewFactory(b))
	sr.AddFactory(handle_webview_viaplugin.NewFactory(b))
	sr.AddFactory(handle_rpc_viaplugin.NewFactory(b))
	sr.AddFactory(web_view_observer.NewFactory(b))
	sr.AddFactory(web_fetch_service.NewFactory(b))
	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(volume_rpc_client.NewFactory(b))
	sr.AddFactory(unixfs_world_access.NewFactory(b))
	sr.AddFactory(web_plugin_handle_web_view.NewFactory(b))
	sr.AddFactory(web_plugin_handle_rpc.NewFactory(b))
	sr.AddFactory(http_lookup.NewFactory(b))
}
