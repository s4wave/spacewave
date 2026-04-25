//go:build !js

package main

import (
	"embed"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
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
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	object_peer "github.com/s4wave/spacewave/db/object/peer"
	blocktype_controller_factory "github.com/s4wave/spacewave/hydra-exp/blocktype/controller-factory"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{blocktype_controller_factory.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{object_peer.NewFactory(b)}
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
	return []controller.Factory{sobject_world_engine.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{space_http_download.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{space_http_export.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{space_sobject.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{space_world_optypes.NewFactory(b)}
}}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{cli.NewCliCommands}

// main is the main entrypoint.
func main() { cli_entrypoint.Main("spacewave-cli", "spacewave", factories, configSets, cliCommands) }
