package world_block_tx

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestWorldState tests forking the world state and building a tx batch.
func TestWorldState(t *testing.T) {
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

	// pass 1: build base mock world
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// add the mock object
	objKey := "tx-test-obj-1"
	sender := tb.Volume.GetPeerID()
	_, err = world_block.BuildMockObject(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// pass 2: test forking it + applying changes
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err == nil {
		_, err = world.MustGetObject(ctx, ws, objKey)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// test forking the state using the tx wrapper
	// create a new tx wrapper, forking the state.
	forkedTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// checkRev asserts a object is at a revision
	checkRev := func(obj world.ObjectState, expected uint64) {
		if err := world.AssertObjectRev(ctx, obj, expected); err != nil {
			t.Fatal(err.Error())
		}
	}

	secondMsg := "hello there #2"
	_, _, err = forkedTx.ApplyWorldOp(
		ctx,
		world_mock.NewMockWorldOp(objKey, secondMsg),
		sender,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// ensure the change was applied to the object
	obj, err := world.MustGetObject(ctx, forkedTx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	checkRev(obj, 2)

	// check the updated field on the object
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		e, err := block_mock.UnmarshalExample(ctx, bcs)
		if err == nil && e.GetMsg() != secondMsg {
			err = errors.Errorf("unexpected block msg field: %s != expected %s", e.GetMsg(), secondMsg)
		}
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// check the transaction set
	txBatch := forkedTx.GetTxBatch()
	if l := len(txBatch.GetTxs()); l != 1 {
		t.Fatalf("expected 1 tx but got %d", l)
	}

	// check the tx
	tx := txBatch.GetTxs()[0]
	if tt := tx.GetTxType(); tt != TxType_TxType_APPLY_WORLD_OP {
		t.Fatalf("expected %s but got %s", TxType_TxType_APPLY_WORLD_OP.String(), tt.String())
	}

	// pass 3: apply the tx to a fresh state and check result
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err == nil {
		_, err = world.MustGetObject(ctx, ws, objKey)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	ttx, err := tx.LocateTx()
	if err == nil {
		_, err = ttx.ExecuteTx(
			ctx,
			sender,
			world_mock.LookupMockOp,
			ws,
		)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// ensure the change was applied to the object
	obj, err = world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	checkRev(obj, 2)

	// wait a moment before finishing the test
	<-time.After(time.Millisecond * 100)
}
