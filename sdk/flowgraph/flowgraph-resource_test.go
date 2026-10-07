//go:build !js

package s4wave_flowgraph_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	space_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
)

// setupFlowgraphClient mounts a real RPC World with two Devices and two actors.
func setupFlowgraphClient(t *testing.T) (*resource_client.Client, *sdk_engine.SDKEngine, srpc.Client) {
	// Start the Resource testbed and register the production object type lookup.
	t.Helper()
	ctx := t.Context()
	tb, client, release := resource_testbed.SetupTestbedWithClient(ctx, t)
	t.Cleanup(release)
	controller := objecttype_controller.NewController(space_objecttypes.LookupObjectType)
	releaseController, err := tb.Bus.AddController(ctx, controller, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseController)

	// Create an RPC World and retain its engine reference through the test.
	root := client.AccessRootResource()
	t.Cleanup(root.Release)
	invoker, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	response, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(invoker).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Wrap the World engine Resource for both raw RPC and SDK access.
	engineRef := client.CreateResourceReference(response.GetResourceId())
	t.Cleanup(engineRef.Release)
	engineInvoker, err := engineRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	engine, err := sdk_engine.NewSDKEngine(client, engineRef)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Release)

	// Seed independently addressed graphs and placement destinations through RPC.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, key := range []string{"device/thumper", "device/laptop", "actor/first", "actor/second"} {
		obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_device.Device{PeerId: key, Label: key}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if key == "device/thumper" || key == "device/laptop" {
			if err := world_types.SetObjectType(ctx, tx, key, s4wave_device.DeviceTypeID); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, key := range []string{"flowgraph/first", "flowgraph/second"} {
		if _, _, err := tx.ApplyWorldOp(ctx, &flowgraph.CreateFlowgraphOp{ObjectKey: key, Name: key}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return client, engine, engineInvoker
}

// mountFlowgraph mounts a graph through production typed object Resource access.
func mountFlowgraph(t *testing.T, client *resource_client.Client, invoker srpc.Client, key string) flowgraph.SRPCFlowgraphResourceServiceClient {
	// Resolve the graph's type and retain its child Resource.
	t.Helper()
	response, err := s4wave_world.AccessTypedObject(t.Context(), s4wave_world.NewSRPCTypedObjectResourceServiceClient(invoker), &s4wave_world.AccessTypedObjectRequest{ObjectKey: key})
	if err != nil {
		t.Fatal(err)
	}
	ref := client.CreateResourceReference(response.GetResourceId())
	t.Cleanup(ref.Release)
	child, err := ref.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	return flowgraph.NewSRPCFlowgraphResourceServiceClient(child)
}

// graphNodes constructs the Device nodes and a two-output actor Step.
func graphNodes() *flowgraph.UpdateFlowgraphRequest {
	return &flowgraph.UpdateFlowgraphRequest{
		SetNodes: map[string]*flowgraph.FlowgraphNode{
			"tcp": {TypeId: flowgraph.TCPPortNodeTypeID, Parameters: map[string]string{"address": "127.0.0.1:8080"}, Ports: []*flowgraph.FlowgraphPort{
				{Name: "stream", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "stream"},
			}},
			"local": {TypeId: flowgraph.LocalPortNodeTypeID, Parameters: map[string]string{"address": "127.0.0.1:8080"}, Ports: []*flowgraph.FlowgraphPort{
				{Name: "stream", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT, TypeId: "stream"},
			}},
			"step": {TypeId: flowgraph.StepNodeTypeID, Step: &flowgraph.FlowgraphStep{Prompt: "Check the result."}, Ports: []*flowgraph.FlowgraphPort{
				{Name: "in", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT, TypeId: "activation"},
				{Name: "pass", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "activation", Condition: "The check passed."},
				{Name: "fail", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "activation", Condition: "The check failed."},
			}},
		},
		SetPlacements: map[string]*flowgraph.FlowgraphPlacement{
			"tcp":   {Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: "device/thumper"},
			"local": {Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: "device/laptop"},
			"step":  {Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR, ObjectKey: "actor/first"},
		},
	}
}

// TestFlowgraphResource checks authored edits, placement watches, and node removal through RPC.
func TestFlowgraphResource(t *testing.T) {
	// Mount the first of two independent Flowgraphs and watch its committed revisions.
	client, _, invoker := setupFlowgraphClient(t)
	service := mountFlowgraph(t, client, invoker, "flowgraph/first")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	// Read the initial revision before any authored change.
	watch, err := service.WatchFlowgraph(ctx, &flowgraph.WatchFlowgraphRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	initial, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}

	// Add Device nodes and an actor Step, then observe that exact authored revision.
	nodes := graphNodes()
	updated, err := service.UpdateFlowgraph(ctx, nodes)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !observed.GetSnapshot().EqualVT(updated.GetSnapshot()) {
		t.Fatal("watch did not read the committed node and placement change")
	}
	if updated.GetSnapshot().GetRevision() <= initial.GetSnapshot().GetRevision() {
		t.Fatal("node edit did not advance revision")
	}

	// Connect the stream endpoints and require complete typed readback.
	connections := map[string]*flowgraph.FlowgraphConnection{
		"forward": {OutputNode: "tcp", OutputPort: "stream", InputNode: "local", InputPort: "stream"},
	}
	connected, err := service.UpdateFlowgraph(ctx, &flowgraph.UpdateFlowgraphRequest{SetConnections: connections})
	if err != nil {
		t.Fatal(err)
	}
	read, err := service.GetFlowgraph(ctx, &flowgraph.GetFlowgraphRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !read.GetSnapshot().EqualVT(connected.GetSnapshot()) {
		t.Fatal("readback differs from the authored connection")
	}

	// Replace an actor edge without retaining the previous destination.
	replaced, err := service.UpdateFlowgraph(ctx, &flowgraph.UpdateFlowgraphRequest{
		SetPlacements: map[string]*flowgraph.FlowgraphPlacement{"step": {Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR, ObjectKey: "actor/second"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.GetSnapshot().GetPlacements()["step"].GetObjectKey() != "actor/second" {
		t.Fatal("actor placement was not replaced")
	}

	// Remove a node together with its placement and incident connection.
	removed, err := service.UpdateFlowgraph(ctx, &flowgraph.UpdateFlowgraphRequest{RemoveNodeIds: []string{"tcp"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.GetSnapshot().GetState().GetConnections()) != 0 || len(removed.GetSnapshot().GetPlacements()) != 2 {
		t.Fatal("node removal retained its connection or placement")
	}

	// Report no history in a World that keeps no changelog.
	history, err := service.ListFlowgraphChanges(ctx, &flowgraph.ListFlowgraphChangesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !history.GetChangelogDisabled() || len(history.GetChanges()) != 0 {
		t.Fatal("history without a changelog did not report it disabled")
	}

	// Leave the other Flowgraph untouched.
	other := mountFlowgraph(t, client, invoker, "flowgraph/second")
	otherRead, err := other.GetFlowgraph(ctx, &flowgraph.GetFlowgraphRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(otherRead.GetSnapshot().GetState().GetNodes()) != 0 {
		t.Fatal("editing one Flowgraph changed another")
	}
}

// TestFlowgraphResourceRejectsInvalidEdits checks rollback and snapshot write authority.
func TestFlowgraphResourceRejectsInvalidEdits(t *testing.T) {
	// Create a populated graph through its Resource.
	client, _, invoker := setupFlowgraphClient(t)
	service := mountFlowgraph(t, client, invoker, "flowgraph/first")
	before, err := service.UpdateFlowgraph(t.Context(), graphNodes())
	if err != nil {
		t.Fatal(err)
	}

	// Reject a reversed connection, an incompatible replacement, and a wrong Device.
	requests := []*flowgraph.UpdateFlowgraphRequest{
		{SetConnections: map[string]*flowgraph.FlowgraphConnection{"wrong": {OutputNode: "local", OutputPort: "stream", InputNode: "tcp", InputPort: "stream"}}},
		{SetNodes: map[string]*flowgraph.FlowgraphNode{"step": {TypeId: flowgraph.TCPPortNodeTypeID}}},
		{SetPlacements: map[string]*flowgraph.FlowgraphPlacement{"tcp": {Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: "actor/first"}}},
	}
	for _, request := range requests {
		if _, err := service.UpdateFlowgraph(t.Context(), request); err == nil {
			t.Fatal("invalid edit succeeded")
		}
		after, err := service.GetFlowgraph(t.Context(), &flowgraph.GetFlowgraphRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if !after.GetSnapshot().EqualVT(before.GetSnapshot()) {
			t.Fatal("rejected edit changed the graph")
		}
	}

	// A read transaction's typed Resource cannot upgrade through its attached engine.
	readTx, err := s4wave_world.NewSRPCEngineResourceServiceClient(invoker).NewTransaction(t.Context(), &s4wave_world.NewTransactionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	readRef := client.CreateResourceReference(readTx.GetResourceId())
	t.Cleanup(readRef.Release)
	readInvoker, err := readRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	readService := mountFlowgraph(t, client, readInvoker, "flowgraph/first")
	if _, err := readService.UpdateFlowgraph(t.Context(), &flowgraph.UpdateFlowgraphRequest{RemoveNodeIds: []string{"tcp"}}); err == nil {
		t.Fatal("read-only mount accepted an edit")
	}
}
