//go:build !js

package main

import (
	"embed"

	aperture_cli "github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_forward_rpc_service "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	plugin_host_process "github.com/s4wave/spacewave/bldr/plugin/host/process"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	bldr_plugin_load "github.com/s4wave/spacewave/bldr/plugin/load"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	bldr_web_bundler_esbuild_compiler "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/compiler"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	web_fetch_service "github.com/s4wave/spacewave/bldr/web/fetch/service"
	web_pkg_fs_controller "github.com/s4wave/spacewave/bldr/web/pkg/fs/controller"
	web_pkg_rpc_server "github.com/s4wave/spacewave/bldr/web/pkg/rpc/server"
	bldr_web_plugin_handle_rpc "github.com/s4wave/spacewave/bldr/web/plugin/handle-rpc"
	bldr_web_plugin_handle_web_pkg_assets "github.com/s4wave/spacewave/bldr/web/plugin/handle-web-pkg-assets"
	bldr_web_plugin_handle_web_view_rpc "github.com/s4wave/spacewave/bldr/web/plugin/handle-web-view-rpc"
	web_view_handler_server "github.com/s4wave/spacewave/bldr/web/view/handler/server"
	bldr_web_view_observer "github.com/s4wave/spacewave/bldr/web/view/observer"
	cli "github.com/s4wave/spacewave/cmd/spacewave/cli"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	spacewave_launcher_controller "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	resource_root_controller "github.com/s4wave/spacewave/core/resource/root/controller"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_http_download "github.com/s4wave/spacewave/core/space/http/download"
	space_http_export "github.com/s4wave/spacewave/core/space/http/export"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world_blocktype "github.com/s4wave/spacewave/core/space/world/blocktype"
	optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_overlay "github.com/s4wave/spacewave/db/block/store/overlay"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_rpc_server "github.com/s4wave/spacewave/db/block/store/rpc/server"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	object_peer "github.com/s4wave/spacewave/db/object/peer"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	cluster_controller "github.com/s4wave/spacewave/forge/cluster/controller"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_lib_git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	websocket "github.com/s4wave/spacewave/net/transport/websocket"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// listenerBrokers are the shared yield and listener-status brokers wired at
// this composition root into the resource listener controller, the root
// resource controller, and the CLI commands.
type listenerBrokers struct {
	yield  *yield_policy.Broker
	status *resource_listener.StatusBroker
}

// newListenerBrokers constructs the process-shared broker pair.
func newListenerBrokers() *listenerBrokers {
	return &listenerBrokers{yield: yield_policy.NewBroker(), status: resource_listener.NewStatusBroker()}
}

// buildFactories wires the shared listener brokers into every consumer.
func buildFactories(brokers *listenerBrokers) []cli_entrypoint.AddFactoryFunc {
	return []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
		return []controller.Factory{auth_method_password.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_plugin_compiler_go.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_plugin_forward_rpc_service.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_plugin_load.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_web_bundler_esbuild_compiler.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_web_bundler_vite_compiler.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_web_plugin_handle_rpc.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_web_plugin_handle_web_pkg_assets.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_web_plugin_handle_web_view_rpc.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{bldr_web_view_observer.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{block_store_bucket.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{block_store_overlay.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{block_store_rpc.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{block_store_rpc_server.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{cdn_world_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{cluster_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{dex_solicit.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{execution_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{forge_lib_git_clone.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{forge_lib_kvtx.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{link_solicit_controller.NewFactory()}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{lookup_concurrent.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{manifest_fetch_world.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{object_peer.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{optypes.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{pass_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{peer_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{plugin_host_configset.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{plugin_host_process.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{plugin_host_scheduler.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{plugin_host_wazero_quickjs.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{plugin_space.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{provider_local.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{provider_spacewave.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{resource_listener.NewFactory(b, resource_listener.WithYieldBroker(brokers.yield), resource_listener.WithStatusBroker(brokers.status))}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{resource_root_controller.NewFactory(b, resource_root_controller.WithYieldBroker(brokers.yield), resource_root_controller.WithListenerStatusBroker(brokers.status))}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{session_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{signaling_rpc_client.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{sobject_world_engine.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{space_http_download.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{space_http_export.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{space_sobject.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{space_world_blocktype.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{spacewave_launcher_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{storage_volume.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{task_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{trace_service.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{unixfs_access_http.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{volume_rpc_server.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{web_fetch_service.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{web_pkg_fs_controller.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{web_pkg_rpc_server.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{web_view_handler_server.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{webrtc.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{websocket.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{worker_controller.NewFactory(b)}
	}}
}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// buildCliCommands builds the CLI command builders. Each command process
// owns a private broker pair: it never shares listener state with the
// daemon, and its bus's configured resource listener must not displace the
// foreground serve process; serve binds the socket explicitly after
// installing its own handoff guard.
func buildCliCommands(brokers *listenerBrokers) []cli_entrypoint.BuildCommandsFunc {
	return []cli_entrypoint.BuildCommandsFunc{func(getBus func() cli_entrypoint.CliBus) []*aperture_cli.Command {
		handedOff := false
		protectedGetBus := func() cli_entrypoint.CliBus {
			if !handedOff {
				brokers.yield.BeginHandoff("spacewave CLI", "")
				handedOff = true
			}
			return getBus()
		}
		return cli.NewCliCommands(protectedGetBus, brokers.yield)
	}}
}

// main is the main entrypoint.
func main() {
	brokers := newListenerBrokers()
	cli_entrypoint.Main("spacewave", "spacewave", buildFactories(brokers), configSets, buildCliCommands(brokers))
}
