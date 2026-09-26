package plugin_space_runtime

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

func TestBridgeFilterForwardsOnlyInfrastructure(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	parent, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	recorder := newDirectiveRecorder()
	addTestController(t, parent, recorder)

	child, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	addTestController(t, child, bus_bridge.NewBusBridge(parent, bridgeFilter))

	forwarded := []directive.Directive{
		world.NewLookupWorldEngine("engine"),
		world.NewLookupWorldOp("operation", "engine"),
		volume.NewLookupVolume("volume", ""),
		volume.NewBuildObjectStoreAPI("store", "volume"),
		plugin_host_root.NewLookupRoot([]string{"desktop/darwin/arm64"}),
	}
	for _, dir := range forwarded {
		addTestDirective(t, child, dir)
		recorder.waitFor(t, dir)
	}

	// Plugin loads, manifests, hosts, and RPC services stay inside the
	// generation.
	kept := []directive.Directive{
		plugin_host.NewLookupPluginHost(nil),
		bldr_plugin.NewLoadPluginInstanced("plugin", "space-a"),
		bldr_manifest.NewFetchManifest("plugin", nil, nil, 0),
		bifrost_rpc.NewLookupRpcClient(bldr_plugin.SRPCPluginServiceID, "plugin"),
		bifrost_rpc.NewLookupRpcService(bldr_plugin.SRPCPluginHostServiceID, "plugin-host"),
	}
	for _, dir := range kept {
		addTestDirective(t, child, dir)
	}
	recorder.assertNoMore(t)
}

// addTestController adds ctrl to b until the test ends.
func addTestController(t *testing.T, b bus.Bus, ctrl controller.Controller) {
	t.Helper()
	release, err := b.AddController(t.Context(), ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

// addTestDirective adds dir to b until the test ends.
func addTestDirective(t *testing.T, b bus.Bus, dir directive.Directive) {
	t.Helper()
	_, ref, err := b.AddDirective(dir, bus.NewCallbackHandler(nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
}
