//go:build !js

package s4wave_flowgraph_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// TestNodeRequirements round-trips requirements and validates waiting and placed states.
func TestNodeRequirements(t *testing.T) {
	// Reuse the RPC World with real typed Device destinations.
	_, engine, _ := setupFlowgraphClient(t)
	ctx := t.Context()
	node := &s4wave_flowgraph.FlowgraphNode{TypeId: "fixture", Requirements: &s4wave_flowgraph.FlowgraphRequirements{Replicas: 2, MilliCpu: 500, MemoryBytes: 1024, RuntimeBackend: "docker", MatchLabels: map[string]string{"gpu": "true"}}}
	data, err := node.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	var decoded s4wave_flowgraph.FlowgraphNode
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if !node.EqualVT(&decoded) {
		t.Fatal("requirements changed during binary round trip")
	}

	// Preserve the explicit count and the default single-copy contract.
	if decoded.GetRequirements().ReplicaCount() != 2 || (&s4wave_flowgraph.FlowgraphRequirements{}).ReplicaCount() != 1 {
		t.Fatal("replica count did not round trip or default to one")
	}

	// Accept requirements while waiting, then validate the selected Device normally.
	err = world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		if err := s4wave_flowgraph.ValidatePlacement(ctx, ws, &decoded, nil); err != nil {
			return err
		}
		return s4wave_flowgraph.ValidatePlacement(ctx, ws, &decoded, &s4wave_flowgraph.FlowgraphPlacement{Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: "device/thumper"})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNodeHoldsBoundObject checks that a node running on a Device holds its
// bound object across every Flowgraph of the World, and that an unplaced
// binding or a read of the same object holds nothing.
func TestNodeHoldsBoundObject(t *testing.T) {
	// Bind standing nodes in two Flowgraphs to one object.
	_, engine, _ := setupFlowgraphClient(t)
	ctx := t.Context()
	const held = "actor/first"
	onDevice := func(deviceKey string) *s4wave_flowgraph.FlowgraphPlacements {
		return &s4wave_flowgraph.FlowgraphPlacements{Copies: []*s4wave_flowgraph.FlowgraphPlacement{{Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: deviceKey}}}
	}
	update := func(graphKey string, request *s4wave_flowgraph.UpdateFlowgraphRequest) error {
		return world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
			_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, ws, graphKey, request)
			return err
		})
	}
	checkHold := func(graphKey, nodeID string, node *s4wave_flowgraph.FlowgraphNode) error {
		return world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
			return s4wave_flowgraph.CheckHold(ctx, ws, graphKey, nodeID, node)
		})
	}
	standing := &s4wave_flowgraph.FlowgraphNode{TypeId: "fixture", ObjectKey: held}

	// An unplaced binding in the second graph holds nothing.
	if err := update("flowgraph/second", &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"source": {TypeId: s4wave_flowgraph.EventSourceNodeTypeID, ObjectKey: held}}}); err != nil {
		t.Fatal(err)
	}

	// The first graph's placed node takes the hold and runs.
	if err := update("flowgraph/first", &s4wave_flowgraph.UpdateFlowgraphRequest{
		SetNodes:      map[string]*s4wave_flowgraph.FlowgraphNode{"runtime": standing},
		SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"runtime": onDevice("device/thumper")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := checkHold("flowgraph/first", "runtime", standing); err != nil {
		t.Fatal(err)
	}

	// A second admission placement is refused: in another graph, as a second
	// node of the same graph, and as a second copy or replica.
	requirements := standing.CloneVT()
	requirements.Requirements = &s4wave_flowgraph.FlowgraphRequirements{}
	if err := update("flowgraph/second", &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"runtime": requirements}}); err == nil {
		t.Fatal("another graph took a held object")
	}
	if err := update("flowgraph/first", &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"again": requirements}}); err == nil {
		t.Fatal("a second node of the holding graph took the held object")
	}
	twoCopies := onDevice("device/thumper")
	twoCopies.Copies = append(twoCopies.Copies, onDevice("device/laptop").GetCopies()...)
	replicated := requirements.CloneVT()
	replicated.Requirements.Replicas = 2
	if err := update("flowgraph/first", &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"runtime": replicated}}); err == nil {
		t.Fatal("a holding node took two replicas")
	}
	if err := update("flowgraph/first", &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"runtime": requirements}, SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"runtime": twoCopies}}); err == nil {
		t.Fatal("a holding node took two copies")
	}

	// Unplacing the node releases the hold, and the second graph takes it.
	// The first graph's node no longer runs.
	if err := update("flowgraph/first", &s4wave_flowgraph.UpdateFlowgraphRequest{RemovePlacementNodeIds: []string{"runtime"}}); err != nil {
		t.Fatal(err)
	}
	if err := update("flowgraph/second", &s4wave_flowgraph.UpdateFlowgraphRequest{
		SetNodes:      map[string]*s4wave_flowgraph.FlowgraphNode{"runtime": standing},
		SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"runtime": onDevice("device/laptop")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := checkHold("flowgraph/first", "runtime", standing); err == nil {
		t.Fatal("a node that lost its hold still runs")
	}
	if err := checkHold("flowgraph/second", "runtime", standing); err != nil {
		t.Fatal(err)
	}

	// A read holds nothing: beside the holder, it runs on two Devices at once.
	reader := &s4wave_flowgraph.FlowgraphNode{
		TypeId:       "fixture",
		ObjectKey:    held,
		Reads:        true,
		Requirements: &s4wave_flowgraph.FlowgraphRequirements{Replicas: 2},
	}
	if err := update("flowgraph/first", &s4wave_flowgraph.UpdateFlowgraphRequest{
		SetNodes:      map[string]*s4wave_flowgraph.FlowgraphNode{"reader": reader},
		SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"reader": twoCopies},
	}); err != nil {
		t.Fatal(err)
	}
	if err := checkHold("flowgraph/first", "reader", reader); err != nil {
		t.Fatal(err)
	}
}
