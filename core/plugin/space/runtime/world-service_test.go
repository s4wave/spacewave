//go:build !js

package plugin_space_runtime

import (
	"context"
	"testing"
	"time"

	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestWorldServicePluginLookup runs a plugin's World lookup through its host:
// the plugin bus reaches the generation's World service over the plugin host
// RPC, writes to the Space's World, and leaves other engines unresolved.
func TestWorldServicePluginLookup(t *testing.T) {
	// Hold an in-memory Space World on the parent bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Serve the World on a generation bus bridged from the parent.
	child, _, err := controllerbus_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}
	addTestController(t, child, bus_bridge.NewBusBridge(tb.Bus, bridgeFilter(&plugin_space.Config{EngineId: tb.EngineID}, nil)))
	addTestController(t, child, newWorldService(tb.Logger, child, tb.EngineID, ""))

	// Connect a plugin bus to the generation as the plugin host does, naming
	// the plugin in the server ID: the service serves no unnamed lookup.
	pluginBus, _, err := controllerbus_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}
	host := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux(bifrost_rpc.NewInvoker(child, bldr_plugin.PluginServerID("test-plugin", ""), true)))))
	info := controller.NewInfo("test/plugin-host", controller.MustParseVersion("0.0.1"), "")
	addTestController(t, pluginBus, bifrost_rpc.NewClientController(tb.Logger, pluginBus, info, host, []string{bldr_plugin.HostServiceIDPrefix}))
	addTestController(t, pluginBus, sdk_world_engine.NewSpaceWorldController(pluginBus))

	// Look up the Space World as a plugin controller does.
	engine, _, engineRef, err := world.ExLookupWorldEngine(ctx, pluginBus, false, tb.EngineID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engineRef.Release)

	// Commit the object in one transaction.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := tx.CreateObject(ctx, "plugin/written", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The parent's engine sees the write.
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	written, found, err := read.GetObject(ctx, "plugin/written")
	world.ReleaseObjectState(written)
	if err != nil || !found {
		t.Fatalf("parent World lacks the plugin's object: found=%v err=%v", found, err)
	}

	// A lookup of another engine stays unresolved.
	other, _, otherRef, err := world.ExLookupWorldEngine(ctx, pluginBus, true, "other-engine", nil)
	if err != nil {
		t.Fatal(err)
	}
	if otherRef != nil {
		otherRef.Release()
	}
	if other != nil {
		t.Fatal("plugin resolved an engine other than its Space's")
	}
}
