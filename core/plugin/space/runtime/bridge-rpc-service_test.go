package plugin_space_runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// TestBridgeKeepsSpaceWorldServiceInside checks that a plugin in one Space
// cannot resolve another Space's World service. The plugin host allows a Space
// plugin the SpaceWorld service by ID on its own Space's bus, so that service
// must never cross a generation's bridges: bridgeDirective keeps the lookup out
// of the parent and parentDirective keeps a parent lookup out of the Space.
func TestBridgeKeepsSpaceWorldServiceInside(t *testing.T) {
	// Build the parent and two Space buses with the generation's filters.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	parent := newRPCTestBus(t, le)
	spaceA, spaceB := newRPCTestBus(t, le), newRPCTestBus(t, le)
	apps := []string{"app"}
	confA := &plugin_space.Config{EngineId: "engine-a", VolumeId: "volume-a"}
	confB := &plugin_space.Config{EngineId: "engine-b", VolumeId: "volume-b"}
	addTestController(t, spaceA, bus_bridge.NewBusBridge(parent, bridgeFilter(confA, apps)))
	addTestController(t, parent, bus_bridge.NewBusBridge(spaceA, parentFilter("space-a", apps)))
	addTestController(t, spaceB, bus_bridge.NewBusBridge(parent, bridgeFilter(confB, apps)))
	addTestController(t, parent, bus_bridge.NewBusBridge(spaceB, parentFilter("space-b", apps)))

	// Serve a World service on Space B's bus and on the parent.
	serveTestSpaceWorld(t, spaceB, "space-b")
	serveTestSpaceWorld(t, parent, "parent")

	// Each bus resolves its own World service.
	if got := lookupTestSpaceWorld(ctx, t, spaceB); got != "space-b" {
		t.Fatalf("Space B resolved %q, want its own World", got)
	}
	if got := lookupTestSpaceWorld(ctx, t, parent); got != "parent" {
		t.Fatalf("parent resolved %q, want its own World", got)
	}

	// Space A resolves neither Space B's World nor the parent's.
	if got := lookupTestSpaceWorld(ctx, t, spaceA); got != "" {
		t.Fatalf("Space A resolved the World service on %s", got)
	}
}

// TestBridgeSpaceWorldLeakIsVisible checks that the lookup in
// TestBridgeKeepsSpaceWorldServiceInside would see a leak: bridges that
// forward every directive let Space A resolve Space B's World service.
func TestBridgeSpaceWorldLeakIsVisible(t *testing.T) {
	// Forward everything from Space A to the parent and on to Space B.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	parent := newRPCTestBus(t, le)
	spaceA, spaceB := newRPCTestBus(t, le), newRPCTestBus(t, le)
	forwardAll := func(directive.Instance) (bool, error) { return true, nil }
	addTestController(t, spaceA, bus_bridge.NewBusBridge(parent, forwardAll))
	addTestController(t, parent, bus_bridge.NewBusBridge(spaceB, forwardAll))

	// Space A reaches Space B's World service.
	serveTestSpaceWorld(t, spaceB, "space-b")
	if got := lookupTestSpaceWorld(ctx, t, spaceA); got != "space-b" {
		t.Fatalf("Space A resolved %q through open bridges, want space-b", got)
	}
}

// newRPCTestBus constructs a controller bus for the test.
func newRPCTestBus(t *testing.T, le *logrus.Entry) bus.Bus {
	t.Helper()

	// Start a bare core bus bound to the test context.
	b, _, err := controllerbus_core.NewCoreBus(t.Context(), le)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// serveTestSpaceWorld answers SpaceWorld lookups on b with an invoker that
// records busName in its error.
func serveTestSpaceWorld(t *testing.T, b bus.Bus, busName string) {
	t.Helper()

	// Record the bus in every call so a lookup shows which bus served the
	// call.
	invoker := srpc.InvokerFunc(func(string, string, srpc.Stream) (bool, error) {
		return true, &servedOnError{bus: busName}
	})
	ctrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test/space-world/"+busName, controller.MustParseVersion("0.0.1"), "test SpaceWorld service"),
		func(context.Context, func()) (srpc.Invoker, func(), error) { return invoker, nil, nil },
		nil,
		false,
		nil,
		[]string{sdk_world_engine.SpaceWorldServiceID},
		nil,
	)
	addTestController(t, b, ctrl)
}

// servedOnError records the bus whose test service answered a call.
type servedOnError struct {
	bus string
}

// Error returns the error string.
func (e *servedOnError) Error() string {
	return "served on " + e.bus
}

// lookupTestSpaceWorld resolves the SpaceWorld service on b as a Space plugin
// would and returns the bus that served it, or "" when the lookup goes idle
// with no service.
func lookupTestSpaceWorld(ctx context.Context, t *testing.T, b bus.Bus) string {
	t.Helper()

	// Resolve the service as the plugin with Space A's binding key.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	serverID := bldr_plugin.PluginServerID("plugin", "space-a")
	vals, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b, sdk_world_engine.SpaceWorldServiceID, serverID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) == 0 {
		return ""
	}
	defer ref.Release()

	// Call the first value to learn which bus served it.
	if len(vals) != 1 {
		t.Fatalf("resolved %d SpaceWorld services, want one", len(vals))
	}
	_, err = vals[0].InvokeMethod(sdk_world_engine.SpaceWorldServiceID, "Probe", nil)
	var served *servedOnError
	if !errors.As(err, &served) {
		t.Fatalf("SpaceWorld call returned %v", err)
	}
	return served.bus
}
