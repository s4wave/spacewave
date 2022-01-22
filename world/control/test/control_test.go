package control

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
)

// TestWaitForObjectRev tests the control loop.
func TestWaitForObjectRev(t *testing.T) {
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
	select {
	case err := <-errCh:
		t.Fatal(err.Error())
	case res := <-objCh:
		_, outRev, err := res.GetRootRef()
		if err != nil {
			t.Fatal(err.Error())
		}
		if outRev != 2 {
			t.Fatalf("expected rev: %v but got %v", 2, outRev)
		}
	}
}
