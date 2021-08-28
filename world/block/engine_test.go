package world_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
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
		ocs,
		world_mock.GetMockWorldOpHandlers(),
		world_mock.GetMockObjectOpHandlers(),
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

	ws, err := BuildMockWorldState(ctx, ocs)
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
	ws, err = BuildMockWorldState(ctx, ocs)
	if err == nil {
		_, err = world.MustGetObject(ws, objKey)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	forked, err := ws.Fork(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply operation, after, rev=3
	_, err = forked.ApplyWorldOp(
		world_mock.MockWorldOpId,
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
	ws, err = BuildMockWorldState(ctx, ocs)
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
