//go:build !js

package s4wave_flowgraph_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// TestReadFlowgraphChanges checks that each authored edit reads back from the
// World changelog as one revision, in order.
func TestReadFlowgraphChanges(t *testing.T) {
	// Start a World that keeps its changelog.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx, world_testbed.WithChangelog())
	engine := tb.Engine

	// commit applies fn in one committed write transaction.
	commit := func(fn func(tx world.Tx) error) {
		t.Helper()
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		if err := fn(tx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	update := func(key string, request *flowgraph.UpdateFlowgraphRequest) {
		t.Helper()
		commit(func(tx world.Tx) error {
			_, err := flowgraph.UpdateFlowgraph(ctx, tx, key, request)
			return err
		})
	}

	// Seed the placement destinations and two graphs.
	commit(func(tx world.Tx) error {
		for _, key := range []string{"device/thumper", "device/laptop", "actor/first", "actor/second"} {
			obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
				cursor.SetBlock(&s4wave_device.Device{PeerId: key, Label: key}, true)
				return nil
			})
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			if key == "device/thumper" || key == "device/laptop" {
				if err := world_types.SetObjectType(ctx, tx, key, s4wave_device.DeviceTypeID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	for _, key := range []string{"flowgraph/first", "flowgraph/second"} {
		commit(func(tx world.Tx) error {
			_, _, err := tx.ApplyWorldOp(ctx, &flowgraph.CreateFlowgraphOp{ObjectKey: key, Name: key}, "")
			return err
		})
	}

	// Edit nodes, a connection, a placement alone, and a node removal,
	// with an edit to the other graph in between.
	nodes := graphNodes()
	connections := map[string]*flowgraph.FlowgraphConnection{
		"forward": {OutputNode: "tcp", OutputPort: "stream", InputNode: "local", InputPort: "stream"},
	}
	moved := map[string]*flowgraph.FlowgraphPlacement{
		"step": {Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR, ObjectKey: "actor/second"},
	}
	update("flowgraph/first", nodes)
	update("flowgraph/second", graphNodes())
	update("flowgraph/first", &flowgraph.UpdateFlowgraphRequest{SetConnections: connections})
	update("flowgraph/first", &flowgraph.UpdateFlowgraphRequest{SetPlacements: moved})
	update("flowgraph/first", &flowgraph.UpdateFlowgraphRequest{RemoveNodeIds: []string{"tcp"}})

	// Read the history newest first.
	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	history, err := flowgraph.ReadFlowgraphChanges(ctx, read, "flowgraph/first", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Require each edit as one revision, newest first, ending at the creation.
	name := "flowgraph/first"
	want := []*flowgraph.UpdateFlowgraphRequest{
		{RemoveNodeIds: []string{"tcp"}, RemoveConnectionIds: []string{"forward"}, RemovePlacementNodeIds: []string{"tcp"}},
		{SetPlacements: moved},
		{SetConnections: connections},
		nodes,
		{Name: &name},
	}
	changes := history.GetChanges()
	if history.GetChangelogDisabled() || !history.GetComplete() || len(changes) != len(want) {
		t.Fatalf("history has %d changes, want %d from the creation", len(changes), len(want))
	}
	for i, edit := range want {
		if !changes[i].GetEdit().EqualVT(edit) || changes[i].GetCreated() != (i == len(want)-1) {
			t.Fatalf("revision %d is %v, want %v", i, changes[i].GetEdit(), edit)
		}
		if i != 0 && changes[i].GetSeqno() >= changes[i-1].GetSeqno() {
			t.Fatalf("revision %d is not older than revision %d", i, i-1)
		}
	}

	// Require a limit to return the newest revisions only.
	limited, err := flowgraph.ReadFlowgraphChanges(ctx, read, "flowgraph/first", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.GetChanges()) != 2 || limited.GetComplete() || !limited.GetChanges()[1].EqualVT(changes[1]) {
		t.Fatal("limited history differs from the newest revisions")
	}
}
