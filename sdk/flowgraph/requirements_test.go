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
