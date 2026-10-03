package world_block_tx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestWorldState_RenameObject records and replays an object rename transaction.
func TestWorldState_RenameObject(t *testing.T) {
	// Prepare the context and logger for the rename test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the empty World cursor and retain it through replay.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the writable World used to seed the rename test.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the source object and an unrelated graph target.
	oldKey := "tx-rename-old"
	newKey := "tx-rename-new"
	otherKey := "tx-rename-other"
	{
		createdObject, err := world_block.BuildMockObject(ctx, ws, oldKey)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := world_block.BuildMockObject(ctx, ws, otherKey)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Connect the source object to the unrelated target in the World graph.
	oldValue := world.KeyToGraphValue(oldKey).String()
	newValue := world.KeyToGraphValue(newKey).String()
	otherValue := world.KeyToGraphValue(otherKey).String()
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuad(oldValue, "<predicate>", otherValue, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Commit the original World so the rename can be replayed independently.
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// Fork the saved World to record the source object rename.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	forkedTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Rename the source object through the transaction wrapper.
	{
		objectState, err := forkedTx.RenameObject(ctx, oldKey, newKey, false)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Check that the batch contains one object rename transaction.
	txBatch := forkedTx.GetTxBatch()
	if len(txBatch.GetTxs()) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txBatch.GetTxs()))
	}
	tx := txBatch.GetTxs()[0]
	if tt := tx.GetTxType(); tt != TxType_TxType_RENAME_OBJECT {
		t.Fatalf("expected %s, got %s", TxType_TxType_RENAME_OBJECT.String(), tt.String())
	}

	// Replay the recorded object rename against the saved World.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	ttx, err := tx.LocateTx()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := ttx.ExecuteTx(ctx, tb.Volume.GetPeerID(), world_mock.LookupMockOp, ws); err != nil {
		t.Fatal(err.Error())
	}

	// Check that replay replaces the source object key.
	{
		objectState2, found, err := ws.GetObject(ctx, oldKey)
		world.ReleaseObjectState(objectState2)
		if err != nil {
			t.Fatal(err.Error())
		} else if found {
			t.Fatalf("expected old key %q to be absent", oldKey)
		}
	}
	{
		objectState3, found, err := ws.GetObject(ctx, newKey)
		world.ReleaseObjectState(objectState3)
		if err != nil {
			t.Fatal(err.Error())
		} else if !found {
			t.Fatalf("expected new key %q to exist", newKey)
		}
	}

	// Check that replay rewrites the graph subject to the new object key.
	oldQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(oldValue, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	newQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(newValue, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(oldQuads) != 0 || len(newQuads) != 1 {
		t.Fatalf("expected graph subject rewrite, got old=%d new=%d", len(oldQuads), len(newQuads))
	}
}

// TestWorldState_RenameObjectDescendants records descendants as replayable rename transactions.
func TestWorldState_RenameObjectDescendants(t *testing.T) {
	// Prepare the context and logger for the descendant rename test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the descendant World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the empty World cursor and retain it through replay.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the writable World used to seed the descendant rename test.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the source object and its descendant objects.
	oldKeys := []string{"repo-1", "repo-1/workdir", "repo-1/worktree"}
	newKeys := []string{"myrepo", "myrepo/workdir", "myrepo/worktree"}
	for _, key := range oldKeys {
		{
			createdObject, err := world_block.BuildMockObject(ctx, ws, key)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatal(err.Error())
			}
		}
	}

	// Commit the original descendant World for independent replay.
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// Fork the saved World to record descendant object renames.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	forkedTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Rename the source object and descendants through the transaction wrapper.
	{
		objectState, err := forkedTx.RenameObject(ctx, oldKeys[0], newKeys[0], true)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Check that the batch records a rename for every source object.
	txBatch := forkedTx.GetTxBatch()
	if len(txBatch.GetTxs()) != len(oldKeys) {
		t.Fatalf("expected %d txs, got %d", len(oldKeys), len(txBatch.GetTxs()))
	}
	for i, tx := range txBatch.GetTxs() {
		if tt := tx.GetTxType(); tt != TxType_TxType_RENAME_OBJECT {
			t.Fatalf("expected tx %d to be %s, got %s", i, TxType_TxType_RENAME_OBJECT.String(), tt.String())
		}
	}

	// Replay the descendant rename batch against the saved World.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := txBatch.ExecuteTx(ctx, tb.Volume.GetPeerID(), world_mock.LookupMockOp, ws); err != nil {
		t.Fatal(err.Error())
	}

	// Check that replay removes every old object key.
	for _, key := range oldKeys {
		{
			objectState2, found, err := ws.GetObject(ctx, key)
			world.ReleaseObjectState(objectState2)
			if err != nil {
				t.Fatal(err.Error())
			} else if found {
				t.Fatalf("expected old key %q to be absent", key)
			}
		}
	}

	// Check that replay creates every renamed object key.
	for _, key := range newKeys {
		{
			objectState3, found, err := ws.GetObject(ctx, key)
			world.ReleaseObjectState(objectState3)
			if err != nil {
				t.Fatal(err.Error())
			} else if !found {
				t.Fatalf("expected new key %q to exist", key)
			}
		}
	}
}
