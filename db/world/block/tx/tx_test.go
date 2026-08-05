package world_block_tx

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/testbed"
	dbtx "github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
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

	// Build the base mock world state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Seed the base state with a mock object and commit it.
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

	// Reopen the base state before forking and applying changes.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err == nil {
		_, err = world.MustGetObject(ctx, ws, objKey)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Fork the state through the transaction wrapper.
	forkedTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Define the revision assertion used for the fork.
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

	// Verify the forked operation changed the object revision.
	obj, err := world.MustGetObject(ctx, forkedTx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	checkRev(obj, 2)

	// Verify the forked operation changed the object payload.
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

	// Verify the fork recorded exactly one transaction.
	txBatch := forkedTx.GetTxBatch()
	if l := len(txBatch.GetTxs()); l != 1 {
		t.Fatalf("expected 1 tx but got %d", l)
	}

	// Verify the recorded transaction type and sender.
	tx := txBatch.GetTxs()[0]
	if tt := tx.GetTxType(); tt != TxType_TxType_APPLY_WORLD_OP {
		t.Fatalf("expected %s but got %s", TxType_TxType_APPLY_WORLD_OP.String(), tt.String())
	}
	if got := tx.GetTxApplyWorldOp().GetOpSender(); got != sender.String() {
		t.Fatalf("expected world op sender %q, got %q", sender.String(), got)
	}

	// Apply the recorded transaction to a fresh state and verify its result.
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

	objectTx, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	obj, err = world.MustGetObject(ctx, objectTx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, _, err = obj.ApplyObjectOp(ctx, world_mock.NewMockObjectOp("object op sender"), sender)
	if err != nil {
		t.Fatal(err.Error())
	}
	txBatch = objectTx.GetTxBatch()
	if l := len(txBatch.GetTxs()); l != 1 {
		t.Fatalf("expected 1 object tx but got %d", l)
	}
	tx = txBatch.GetTxs()[0]
	if tt := tx.GetTxType(); tt != TxType_TxType_APPLY_OBJECT_OP {
		t.Fatalf("expected %s but got %s", TxType_TxType_APPLY_OBJECT_OP.String(), tt.String())
	}
	if got := tx.GetTxApplyObjectOp().GetOpSender(); got != sender.String() {
		t.Fatalf("expected object op sender %q, got %q", sender.String(), got)
	}

	// wait a moment before finishing the test
	<-time.After(time.Millisecond * 100)
}

// TestWorldStateGetObjectBodiesBatchPageAfterDiscard verifies that discarded
// transaction wrappers reject batched body reads.
func TestWorldStateGetObjectBodiesBatchPageAfterDiscard(t *testing.T) {
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
	txState, err := ForkWorldState(ctx, ws, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	txState.Discard()

	_, _, err = txState.GetObjectBodiesBatchPage(ctx, []string{"discarded"}, world.ObjectBodiesBatchByteBudget)
	if err != dbtx.ErrDiscarded {
		t.Fatalf("GetObjectBodiesBatchPage error = %v, want %v", err, dbtx.ErrDiscarded)
	}
}
