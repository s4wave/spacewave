package world_control_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestWatchLoop tests the control loop and WaitForObjectRev.
func TestWatchLoop(t *testing.T) {
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
	obj1, err := ws.CreateObject(ctx, objKey, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = obj1.IncrementRev(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// expect result
	var outRev uint64
	select {
	case err := <-errCh:
		t.Fatal(err.Error())
	case res := <-objCh:
		_, outRev, err = res.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if outRev != 2 {
			t.Fatalf("expected rev: %v but got %v", 2, outRev)
		}
	}

	revCh := make(chan uint64, 10)
	loop := world_control.NewWatchLoop(
		le,
		objKey,
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			obj world.ObjectState,
			rootCs *block.Cursor,
			rev uint64,
		) (bool, error) {
			revCh <- rev
			return true, nil
		}),
	)
	go func() {
		_ = loop.Execute(ctx, ws)
	}()

	// expect initial revision
	<-revCh

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

func TestWatchLoopWakeBeforeWaitIsSticky(t *testing.T) {
	ctx := t.Context()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	revCh := make(chan uint64, 2)
	loop := world_control.NewWatchLoop(
		tb.Logger,
		"",
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			rev uint64,
		) (bool, error) {
			revCh <- rev
			return true, nil
		}),
	)
	loop.Wake()
	go func() {
		_ = loop.Execute(ctx, tb.WorldState)
	}()

	for i := range 2 {
		select {
		case <-revCh:
		case <-time.After(time.Second):
			t.Fatalf("expected sticky wake iteration %d", i+1)
		}
	}
}

func TestWatchLoopWakeAfterWaitClearIsSticky(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	events := make(chan struct{}, 4)
	release := make(chan struct{})
	loop := world_control.NewWatchLoop(
		tb.Logger,
		"",
		world_control.NewWaitForStateHandler(func(
			ctx context.Context,
			_ world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			_ uint64,
		) (bool, error) {
			events <- struct{}{}
			select {
			case <-release:
				return true, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- loop.Execute(ctx, tb.WorldState)
	}()

	recvWatchLoopEvent(t, events, "initial handler")
	release <- struct{}{}

	if _, err := tb.WorldState.CreateObject(ctx, "wake-after-clear", nil); err != nil {
		t.Fatal(err.Error())
	}
	recvWatchLoopEvent(t, events, "world-change handler")

	loop.Wake()
	release <- struct{}{}
	recvWatchLoopEvent(t, events, "sticky wake handler")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit")
	}
}

func TestWatchLoopReportsObjectDeletion(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "delete-object"
	if _, err := tb.WorldState.CreateObject(ctx, objKey, nil); err != nil {
		t.Fatal(err.Error())
	}

	foundCh := make(chan bool, 4)
	loop := world_control.NewWatchLoop(
		tb.Logger,
		objKey,
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			obj world.ObjectState,
			_ *block.Cursor,
			_ uint64,
		) (bool, error) {
			foundCh <- obj != nil
			return true, nil
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- loop.Execute(ctx, tb.WorldState)
	}()

	if found := recvWatchLoopValue(t, foundCh, "initial object state"); !found {
		t.Fatal("expected initial object state")
	}
	deleted, err := tb.WorldState.DeleteObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("object was not deleted")
	}
	if found := recvWatchLoopValue(t, foundCh, "deleted object state"); found {
		t.Fatal("expected deleted object state")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit")
	}
}

func TestWatchLoopCancellationDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	events := make(chan struct{}, 1)
	loop := world_control.NewWatchLoop(
		tb.Logger,
		"",
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			_ uint64,
		) (bool, error) {
			events <- struct{}{}
			return true, nil
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- loop.Execute(ctx, tb.WorldState)
	}()

	recvWatchLoopEvent(t, events, "initial handler")
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Execute err = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit")
	}
}
func TestWatchLoopSkipsUnhandledOperation(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	calls := make(chan int, 2)
	count := 0
	loop := world_control.NewWatchLoop(
		tb.Logger,
		"",
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			_ uint64,
		) (bool, error) {
			count++
			calls <- count
			if count == 1 {
				return false, world.ErrUnhandledOp
			}
			return false, nil
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- loop.Execute(ctx, tb.WorldState)
	}()

	recvWatchLoopValue(t, calls, "initial handler")
	loop.Wake()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute err = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch loop did not continue after unhandled operation")
	}
}

func recvWatchLoopEvent(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func recvWatchLoopValue[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()
	select {
	case val := <-ch:
		return val
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
	var zero T
	return zero
}
