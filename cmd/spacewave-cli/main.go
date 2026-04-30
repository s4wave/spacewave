//go:build !js

package main

import (
	"embed"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	cli "github.com/s4wave/spacewave/cmd/spacewave-cli/cli"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	resource_root_controller "github.com/s4wave/spacewave/core/resource/root/controller"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_http_download "github.com/s4wave/spacewave/core/space/http/download"
	space_http_export "github.com/s4wave/spacewave/core/space/http/export"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	blocktype_controller_factory "github.com/s4wave/spacewave/db/blocktype/controller-factory"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	object_peer "github.com/s4wave/spacewave/db/object/peer"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
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

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{auth_method_password.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{blocktype_controller_factory.NewFactory(b)}
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
	return []controller.Factory{object_peer.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{optypes.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{pass_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{peer_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{plugin_space.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{provider_local.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{provider_spacewave.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{resource_listener.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{resource_root_controller.NewFactory(b)}
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
	return []controller.Factory{storage_volume.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{task_controller.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{unixfs_access_http.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{webrtc.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{websocket.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{worker_controller.NewFactory(b)}
}}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{cli.NewCliCommands}

// main is the main entrypoint.
func main() { cli_entrypoint.Main("spacewave-cli", "spacewave", factories, configSets, cliCommands) }
