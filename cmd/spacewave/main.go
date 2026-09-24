//go:build !js

package main

import (
	"embed"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	bldr_manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	manifest_fetch_plugin "github.com/s4wave/spacewave/bldr/manifest/fetch/plugin"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	bldr_manifest_materializer "github.com/s4wave/spacewave/bldr/manifest/materializer"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_plugin_forward_rpc_service "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	bldr_plugin_handle_web_view "github.com/s4wave/spacewave/bldr/plugin/handle-web-view"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	plugin_host_process "github.com/s4wave/spacewave/bldr/plugin/host/process"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	bldr_plugin_load "github.com/s4wave/spacewave/bldr/plugin/load"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_project_watcher "github.com/s4wave/spacewave/bldr/project/watcher"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	bldr_web_bundler_esbuild_compiler "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/compiler"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	web_fetch_service "github.com/s4wave/spacewave/bldr/web/fetch/service"
	bldr_web_pkg_compiler "github.com/s4wave/spacewave/bldr/web/pkg/compiler"
	web_pkg_fs_controller "github.com/s4wave/spacewave/bldr/web/pkg/fs/controller"
	web_pkg_rpc_client "github.com/s4wave/spacewave/bldr/web/pkg/rpc/client"
	web_pkg_rpc_server "github.com/s4wave/spacewave/bldr/web/pkg/rpc/server"
	bldr_web_plugin_compiler "github.com/s4wave/spacewave/bldr/web/plugin/compiler"
	bldr_web_plugin_controller "github.com/s4wave/spacewave/bldr/web/plugin/controller"
	electron "github.com/s4wave/spacewave/bldr/web/plugin/electron"
	bldr_web_plugin_handle_rpc "github.com/s4wave/spacewave/bldr/web/plugin/handle-rpc"
	bldr_web_plugin_handle_web_pkg_assets "github.com/s4wave/spacewave/bldr/web/plugin/handle-web-pkg-assets"
	bldr_web_plugin_handle_web_pkg_rpc "github.com/s4wave/spacewave/bldr/web/plugin/handle-web-pkg-rpc"
	bldr_web_plugin_handle_web_view_rpc "github.com/s4wave/spacewave/bldr/web/plugin/handle-web-view-rpc"
	saucer "github.com/s4wave/spacewave/bldr/web/plugin/saucer"
	web_view_handler_controller "github.com/s4wave/spacewave/bldr/web/view/handler/controller"
	web_view_handler_server "github.com/s4wave/spacewave/bldr/web/view/handler/server"
	bldr_web_view_observer "github.com/s4wave/spacewave/bldr/web/view/observer"
	project_compose "github.com/s4wave/spacewave/cmd/spacewave/compose"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_http_download "github.com/s4wave/spacewave/core/space/http/download"
	space_http_export "github.com/s4wave/spacewave/core/space/http/export"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world_blocktype "github.com/s4wave/spacewave/core/space/world/blocktype"
	optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_rpc_lookup "github.com/s4wave/spacewave/db/block/store/rpc/lookup"
	block_store_rpc_server "github.com/s4wave/spacewave/db/block/store/rpc/server"
	block_store_rpc_server_bucket "github.com/s4wave/spacewave/db/block/store/rpc/server/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	object_peer "github.com/s4wave/spacewave/db/object/peer"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	cluster_controller "github.com/s4wave/spacewave/forge/cluster/controller"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_lib_git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	stream_api_accept "github.com/s4wave/spacewave/net/stream/api/accept"
	stream_srpc_server_lookup "github.com/s4wave/spacewave/net/stream/srpc/server/lookup"
	inproc "github.com/s4wave/spacewave/net/transport/inproc"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	websocket "github.com/s4wave/spacewave/net/transport/websocket"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{auth_method_password.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_cli_compiler.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_dist_compiler.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_manifest_builder_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_manifest_materializer.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_plugin_compiler_go.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_plugin_compiler_js.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_plugin_forward_rpc_service.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_plugin_handle_web_view.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_plugin_load.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_project_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_project_watcher.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_bundler_esbuild_compiler.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_bundler_vite_compiler.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_pkg_compiler.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_plugin_compiler.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_plugin_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_plugin_handle_rpc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_plugin_handle_web_pkg_assets.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_plugin_handle_web_pkg_rpc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_plugin_handle_web_view_rpc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_web_view_observer.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{block_store_bucket.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{block_store_rpc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{block_store_rpc_lookup.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{block_store_rpc_server.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{block_store_rpc_server_bucket.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bucket_setup.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{cluster_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{dex_solicit.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{electron.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{execution_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{forge_lib_git_clone.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{forge_lib_kvtx.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{inproc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{link_holdopen_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{link_solicit_controller.NewFactory()}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{lookup_concurrent.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{manifest_fetch_plugin.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{manifest_fetch_world.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{node_controller.NewFactory(b)}
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
	return []controller.Factory{saucer.NewFactory(b)}
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
	return []controller.Factory{storage_volume.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{stream_api_accept.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{stream_srpc_server_lookup.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{task_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{unixfs_access_http.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{volume_bolt.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{volume_kvtxinmem.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{volume_rpc_client.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{volume_rpc_server.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{volume_sqlite.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{volume_world.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{web_fetch_service.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{web_pkg_fs_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{web_pkg_rpc_client.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{web_pkg_rpc_server.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{web_view_handler_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{web_view_handler_server.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{webrtc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{websocket.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{worker_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{world_block_engine.NewFactory(b)}
}}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{}

// main is the main entrypoint.
func main() {
	composition := project_compose.Compose()
	cli_entrypoint.Main("spacewave", "spacewave", append(factories, composition.Factories...), configSets, append(cliCommands, composition.Commands...))
}
