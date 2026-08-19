package resource_space

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

func TestSpaceRuntimeBridgeAdmitsOnlyParentInfrastructure(t *testing.T) {
	allowed := []directive.Directive{
		world.NewLookupWorldEngine("engine"),
		world.NewLookupWorldOp("operation", "engine"),
		volume.NewLookupVolume("volume", ""),
		volume.NewBuildObjectStoreAPI("store", "volume"),
		plugin_host_root.NewLookupRoot([]string{"desktop/darwin/arm64"}),
	}
	for _, dir := range allowed {
		if !spaceRuntimeBridgeDirective(dir) {
			t.Fatalf("%T was not bridged", dir)
		}
	}

	blocked := []directive.Directive{
		plugin_host.NewLookupPluginHost(nil),
		bldr_plugin.NewLoadPluginInstanced("plugin", "space-a"),
		bldr_manifest.NewFetchManifest("plugin", nil, nil, 0),
		bifrost_rpc.NewLookupRpcClient(bldr_plugin.SRPCPluginServiceID, "plugin"),
		bifrost_rpc.NewLookupRpcService(bldr_plugin.SRPCPluginHostServiceID, "plugin-host"),
	}
	for _, dir := range blocked {
		if spaceRuntimeBridgeDirective(dir) {
			t.Fatalf("%T leaked to the parent bus", dir)
		}
	}
}
