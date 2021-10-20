package world_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestWorldEngine performs a simple test of operations against world engine.
func TestWorldEngine(t *testing.T) {
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

	eng, err := NewEngine(
		ctx,
		le,
		ocs,
		world_mock.LookupMockOp,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// basic sanity tests
	err = world_mock.TestWorldEngine_Basic(ctx, le, eng)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}

// TestWorldEngine_Fork tests forking the block-backed world state.
//
// Applies the result to the original WorldState & checks.
func TestWorldEngine_Fork(t *testing.T) {
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

	ws, err := BuildMockWorldState(ctx, le, true, ocs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// add the mock object
	objKey := "tx-test-obj-1"
	obj, err := BuildMockObject(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit()
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// test forking it + applying changes
	sender := tb.Volume.GetPeerID()
	ws, err = BuildMockWorldState(ctx, le, true, ocs)
	if err == nil {
		_, err = world.MustGetObject(ws, objKey)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	forkedWs, err := ws.Fork(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	forked := forkedWs.(*WorldState)

	// apply operation, after, rev=3
	_, _, err = forked.ApplyWorldOp(
		world_mock.NewMockWorldOp(objKey, "hello there #2"),
		sender,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// checkRev asserts a object is at a revision
	checkRev := func(obj world.ObjectState, expected uint64) {
		if err := world.AssertObjectRev(obj, expected); err != nil {
			t.Fatal(err.Error())
		}
	}

	// ensure original state was still at rev=1
	obj, err = world.MustGetObject(ws, objKey)
	if err == nil {
		checkRev(obj, 1)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// write the forked state
	err = forked.Commit()
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply the updated ref to the original state.
	ocs.SetRootRef(forked.GetRootRef())
	// note: we need a new block transaction to force a new cursor
	ws, err = BuildMockWorldState(ctx, le, true, ocs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check if it was applied
	obj, err = world.MustGetObject(ws, objKey)
	if err == nil {
		checkRev(obj, 2)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}

// TestWorldEngine_UpdateRootRef tests updating the root ref while a write tx is active.
func TestWorldEngine_UpdateRootRef(t *testing.T) {
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

	eng, err := NewEngine(
		ctx,
		le,
		ocs,
		world_mock.LookupMockOp,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-object"

	// create the object in the world
	ws, err := eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	_, err = ws.CreateObject(objKey, oref1)
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// save the first state
	state1 := eng.GetRootRef()

	// change the object
	ws, err = eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	obj1, err := world.MustGetObject(ws, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	rev2, err := obj1.IncrementRev()
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a new read tx
	rtx, err := eng.NewTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// save the second state
	state2 := eng.GetRootRef()

	// ensure the rev is correct
	obj1, err = world.MustGetObject(rtx, objKey)
	if err == nil {
		var rev uint64
		_, rev, err = obj1.GetRootRef()
		if err == nil && rev != rev2 {
			err = errors.Errorf("expected rev %d but got %d", rev2, rev)
		}
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a write tx
	wtx, err := eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// change back to the original state
	err = eng.SetRootRef(ctx, state1)
	// use the same read tx to get the current rev
	if err == nil {
		var rev uint64
		_, rev, err = obj1.GetRootRef()
		if err == nil && rev != rev2-1 {
			err = errors.Errorf("expected rev %d - 1 = %d but got %d", rev2, rev2-1, rev)
		}
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	// expect the write tx to have been discarded
	werr := wtx.Commit(ctx)
	if werr != tx.ErrDiscarded {
		t.Fatalf("expected discarded error, got %v", werr)
	}
	ws.Discard()

	// could check state2 again as well
	_ = state2

	// success
	t.Log("tests successful")
}
