package world_block_tx

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/sirupsen/logrus"
)

// TestWorldState_RenameObject records and replays an object rename transaction.
func TestWorldState_RenameObject(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	oldKey := "tx-rename-old"
	newKey := "tx-rename-new"
	otherKey := "tx-rename-other"
	if _, err := world_block.BuildMockObject(ctx, ws, oldKey); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := world_block.BuildMockObject(ctx, ws, otherKey); err != nil {
		t.Fatal(err.Error())
	}
	oldValue := world.KeyToGraphValue(oldKey).String()
	newValue := world.KeyToGraphValue(newKey).String()
	otherValue := world.KeyToGraphValue(otherKey).String()
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuad(oldValue, "<predicate>", otherValue, "")); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	forkedTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := forkedTx.RenameObject(ctx, oldKey, newKey, false); err != nil {
		t.Fatal(err.Error())
	}
	txBatch := forkedTx.GetTxBatch()
	if len(txBatch.GetTxs()) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txBatch.GetTxs()))
	}
	tx := txBatch.GetTxs()[0]
	if tt := tx.GetTxType(); tt != TxType_TxType_RENAME_OBJECT {
		t.Fatalf("expected %s, got %s", TxType_TxType_RENAME_OBJECT.String(), tt.String())
	}

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

	if _, found, err := ws.GetObject(ctx, oldKey); err != nil {
		t.Fatal(err.Error())
	} else if found {
		t.Fatalf("expected old key %q to be absent", oldKey)
	}
	if _, found, err := ws.GetObject(ctx, newKey); err != nil {
		t.Fatal(err.Error())
	} else if !found {
		t.Fatalf("expected new key %q to exist", newKey)
	}
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
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	oldKeys := []string{"repo-1", "repo-1/workdir", "repo-1/worktree"}
	newKeys := []string{"myrepo", "myrepo/workdir", "myrepo/worktree"}
	for _, key := range oldKeys {
		if _, err := world_block.BuildMockObject(ctx, ws, key); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	forkedTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := forkedTx.RenameObject(ctx, oldKeys[0], newKeys[0], true); err != nil {
		t.Fatal(err.Error())
	}
	txBatch := forkedTx.GetTxBatch()
	if len(txBatch.GetTxs()) != len(oldKeys) {
		t.Fatalf("expected %d txs, got %d", len(oldKeys), len(txBatch.GetTxs()))
	}
	for i, tx := range txBatch.GetTxs() {
		if tt := tx.GetTxType(); tt != TxType_TxType_RENAME_OBJECT {
			t.Fatalf("expected tx %d to be %s, got %s", i, TxType_TxType_RENAME_OBJECT.String(), tt.String())
		}
	}

	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := txBatch.ExecuteTx(ctx, tb.Volume.GetPeerID(), world_mock.LookupMockOp, ws); err != nil {
		t.Fatal(err.Error())
	}
	for _, key := range oldKeys {
		if _, found, err := ws.GetObject(ctx, key); err != nil {
			t.Fatal(err.Error())
		} else if found {
			t.Fatalf("expected old key %q to be absent", key)
		}
	}
	for _, key := range newKeys {
		if _, found, err := ws.GetObject(ctx, key); err != nil {
			t.Fatal(err.Error())
		} else if !found {
			t.Fatalf("expected new key %q to exist", key)
		}
	}
}
