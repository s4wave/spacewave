package world_control_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
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

func TestWatchLoopSkipsCanceledShutdownWarning(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.WarnLevel)
	loop := world_control.NewWatchLoop(
		logrus.NewEntry(logger),
		"",
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			_ uint64,
		) (bool, error) {
			cancel()
			return false, errors.Join(context.Canceled, errors.New("handler exit"))
		}),
	)

	if err := loop.Execute(ctx, tb.WorldState); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute err = %v, want context.Canceled", err)
	}
	if entries := hook.AllEntries(); len(entries) != 0 {
		t.Fatalf("warning count = %d, want 0", len(entries))
	}
}
func TestWatchLoopSkipsUnhandledOperation(t *testing.T) {
	ctx := t.Context()
	ws := setupRemoteWorldState(ctx, t)

	calls := make(chan int, 2)
	count := 0
	loop := world_control.NewWatchLoop(
		logrus.NewEntry(logrus.New()),
		"",
		world_control.NewWaitForStateHandler(func(
			ctx context.Context,
			state world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			_ uint64,
		) (bool, error) {
			count++
			calls <- count
			if count == 1 {
				_, _, err := state.ApplyWorldOp(ctx, &unhandledWorldOp{}, peer.ID(""))
				if !errors.Is(err, world.ErrUnhandledOp) {
					t.Errorf("remote ApplyWorldOp error = %v, want world.ErrUnhandledOp", err)
				}
				return false, err
			}
			return false, nil
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- loop.Execute(ctx, ws)
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

func setupRemoteWorldState(ctx context.Context, t *testing.T) world.WorldState {
	t.Helper()

	_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
	t.Cleanup(cleanup)

	rootRef := resClient.AccessRootResource()
	t.Cleanup(rootRef.Release)
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}

	engineRef := resClient.CreateResourceReference(createResp.ResourceId)
	t.Cleanup(engineRef.Release)
	engine, err := sdk_world_engine.NewSDKEngine(resClient, engineRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(engine.Release)
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tx.Discard)
	return tx
}

type unhandledWorldOp struct{}

func (*unhandledWorldOp) MarshalBlock() ([]byte, error) {
	return nil, nil
}

func (*unhandledWorldOp) UnmarshalBlock([]byte) error {
	return nil
}

func (*unhandledWorldOp) Validate() error {
	return nil
}

func (*unhandledWorldOp) GetOperationTypeId() string {
	return "test/unhandled-world-op"
}

func (*unhandledWorldOp) ApplyWorldOp(
	context.Context,
	*logrus.Entry,
	world.WorldState,
	peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
}

func (*unhandledWorldOp) ApplyWorldObjectOp(
	context.Context,
	*logrus.Entry,
	world.ObjectState,
	peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
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

// TestWatchLoopReleasesObjectStatePerIteration proves the loop hands back every
// object handle it takes. A remote world state allocates a server-side resource
// per GetObject, so a loop that watched a busy object for minutes used to leave
// one tracked handle behind per revision.
func TestWatchLoopReleasesObjectStatePerIteration(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	remote := setupRemoteWorldState(ctx, t)
	objKey := "release-object"
	obj, err := remote.CreateObject(ctx, objKey, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	ws := &countingWorldState{WorldState: remote}
	revs := make(chan uint64, 8)
	loop := world_control.NewWatchLoop(
		logrus.NewEntry(logrus.New()),
		objKey,
		world_control.NewWaitForStateHandler(func(
			_ context.Context,
			_ world.WorldState,
			_ world.ObjectState,
			_ *block.Cursor,
			rev uint64,
		) (bool, error) {
			revs <- rev
			return true, nil
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- loop.Execute(ctx, ws)
	}()

	const revisions = 3
	for i := 0; i <= revisions; i++ {
		recvWatchLoopValue(t, revs, "handler call")
		if outstanding := ws.outstanding.Load(); outstanding > 1 {
			t.Fatalf("outstanding object handles = %d, want at most 1", outstanding)
		}
		if i == revisions {
			break
		}
		if _, err := obj.IncrementRev(ctx); err != nil {
			t.Fatal(err.Error())
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit")
	}

	if acquired := ws.acquired.Load(); acquired < revisions {
		t.Fatalf("acquired object handles = %d, want at least %d", acquired, revisions)
	}
	if outstanding := ws.outstanding.Load(); outstanding != 0 {
		t.Fatalf("outstanding object handles after exit = %d, want 0", outstanding)
	}
}

// countingWorldState counts the object handles taken from a real world state
// and the ones handed back.
type countingWorldState struct {
	world.WorldState

	acquired    atomic.Int64
	outstanding atomic.Int64
}

func (c *countingWorldState) GetObject(ctx context.Context, objKey string) (world.ObjectState, bool, error) {
	obj, found, err := c.WorldState.GetObject(ctx, objKey)
	if obj == nil {
		return obj, found, err
	}
	c.acquired.Add(1)
	c.outstanding.Add(1)
	return &countingObjectState{ObjectState: obj, ws: c}, found, err
}

// countingObjectState reports its release to the owning countingWorldState.
type countingObjectState struct {
	world.ObjectState

	ws       *countingWorldState
	released atomic.Bool
}

func (c *countingObjectState) Release() {
	if !c.released.Swap(true) {
		c.ws.outstanding.Add(-1)
	}
	world.ReleaseObjectState(c.ObjectState)
}
