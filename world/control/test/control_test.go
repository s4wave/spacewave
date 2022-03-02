package control

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
)

// TestControlLoop tests the control loop and WaitForObjectRev.
func TestControlLoop(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	le := tb.Logger
	ws := tb.WorldState
	objKey := "test-object"

	objCh := make(chan world.ObjectState, 1)
	errCh := make(chan error, 1)
	go func() {
		objs, err := world_control.WaitForObjectRev(ctx, le, ws, objKey, 2)
		if err != nil {
			errCh <- err
			return
		}
		objCh <- objs
	}()

	// perform a couple revisions
	obj1, err := ws.CreateObject(objKey, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = obj1.IncrementRev()
	if err != nil {
		t.Fatal(err.Error())
	}

	// expect result
	var outRev uint64
	select {
	case err := <-errCh:
		t.Fatal(err.Error())
	case res := <-objCh:
		_, outRev, err = res.GetRootRef()
		if err != nil {
			t.Fatal(err.Error())
		}
		if outRev != 2 {
			t.Fatalf("expected rev: %v but got %v", 2, outRev)
		}
	}

	revCh := make(chan uint64, 10)
	loop := world_control.NewObjectLoop(
		le,
		ws,
		false,
		objKey,
		world_control.NewWaitForStateHandler(func(
			obj world.ObjectState,
			rootCs *block.Cursor,
			rev uint64,
		) (bool, error) {
			revCh <- rev
			return true, nil
		}),
	)
	go func() {
		_ = loop.Execute(ctx)
	}()

	// expect initial revision
	_ = <-revCh

	// expect nothing
	select {
	case <-revCh:
		t.Fatal("expected loop to sleep after initial rev")
	case <-time.After(time.Millisecond * 50):
	}

	// trigger wake
	loop.Wake()

	// expect value
	nrev := <-revCh
	if nrev != outRev {
		t.Fatalf("expected new rev %d to be equal to old %d", nrev, outRev)
	}
}
