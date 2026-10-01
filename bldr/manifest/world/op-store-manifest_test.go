package bldr_manifest_world

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestExStoreManifestOpSkipsStoredManifest checks that storing an already
// stored and linked manifest applies no World operation, and that a missing
// link still applies one.
func TestExStoreManifestOpSkipsStoredManifest(t *testing.T) {
	// Start a testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build the mock World state from an empty cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()
	mockWS, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create two manifest stores and count operations applied to the World.
	const hostKey, otherKey = "plugin-host", "other-host"
	for _, key := range []string{hostKey, otherKey} {
		if _, err := CreateManifestStore(ctx, mockWS, key); err != nil {
			t.Fatal(err.Error())
		}
	}
	ws := &opCountingWorldState{WorldState: mockWS}

	// Store the manifest once, then store it again unchanged.
	ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 3)
	const manifestKey = "plugin-host/spacewave-web"
	for range 2 {
		if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, ref); err != nil {
			t.Fatal(err.Error())
		}
	}
	if ws.applied != 1 {
		t.Fatalf("applied %d operations storing one manifest twice, want 1", ws.applied)
	}

	// A new link applies an operation for the stored manifest.
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey, otherKey}, ref); err != nil {
		t.Fatal(err.Error())
	}
	if ws.applied != 2 {
		t.Fatalf("applied %d operations after adding a link, want 2", ws.applied)
	}
}

// opCountingWorldState counts the operations applied to a WorldState.
type opCountingWorldState struct {
	world.WorldState
	// applied is the number of ApplyWorldOp calls.
	applied int
}

// ApplyWorldOp counts op and applies it to the wrapped WorldState.
func (w *opCountingWorldState) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	w.applied++
	return w.WorldState.ApplyWorldOp(ctx, op, sender)
}
