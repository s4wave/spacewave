package world_block

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestEngineKeyWatchWakesOnlyChangedKeys checks that a commit wakes the
// waiters of the objects it changed and leaves other waiters asleep.
func TestEngineKeyWatchWakesOnlyChangedKeys(t *testing.T) {
	// Create two objects on a real block Engine.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx)
	ws := world.NewEngineWorldState(e, true)
	createKeyWatchTestObject(t, ctx, ws, "watch/quiet")
	createKeyWatchTestObject(t, ctx, ws, "watch/busy")

	// Register a waiter on the quiet object.
	locked := e.bcast.Lock()
	quietWatch, quietWake, err := e.watchObjectKeyLocked("watch/quiet")
	if err != nil {
		locked.Unlock()
		t.Fatal(err)
	}

	// Register a waiter on the busy object and capture both wake channels.
	busyWatch, busyWake, err := e.watchObjectKeyLocked("watch/busy")
	if err != nil {
		locked.Unlock()
		t.Fatal(err)
	}
	quietCh, busyCh := quietWake.ch, busyWake.ch
	locked.Unlock()
	defer e.unwatchObjectKey(quietWatch, "watch/quiet", quietWake)
	defer e.unwatchObjectKey(busyWatch, "watch/busy", busyWake)

	// One comparison wakes every changed key under one lock, so the busy
	// waiter's wakeup proves the quiet key was compared and left unchanged.
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("watch/busy", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-busyCh:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-quietCh:
		t.Fatal("a commit to another object woke the quiet waiter")
	default:
	}

	// A commit to the quiet object wakes its waiter.
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("watch/quiet", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-quietCh:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// TestEngineWaitObjectRev checks that a revision wait returns after its
// object changes, waits for a missing object, and ends when the Engine closes.
func TestEngineWaitObjectRev(t *testing.T) {
	// Create the watched object on a real block Engine.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx)
	ws := world.NewEngineWorldState(e, true)
	rev := createKeyWatchTestObject(t, ctx, ws, "watch/object")

	// A satisfied revision returns at once and a missing object is rejected.
	got, err := e.WaitObjectRev(ctx, "watch/object", rev, false)
	if err != nil || got != rev {
		t.Fatalf("WaitObjectRev(current) = %d, %v; want %d", got, err, rev)
	}
	if _, err := e.WaitObjectRev(ctx, "watch/missing", 0, false); !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("WaitObjectRev(missing) error = %v, want ErrObjectNotFound", err)
	}

	// A waiter for the next revision returns once the object changes.
	done := make(chan error, 1)
	go func() {
		next, err := e.WaitObjectRev(ctx, "watch/object", rev+1, false)
		if err == nil && next <= rev {
			err = errors.Errorf("revision %d, want greater than %d", next, rev)
		}
		done <- err
	}()
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("watch/object", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// A waiter for an object that never appears ends when the Engine closes.
	go func() {
		_, err := e.WaitObjectRev(ctx, "watch/missing", 1, true)
		done <- err
	}()
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("WaitObjectRev after Close error = %v, want ErrEngineClosed", err)
	}
}

// createKeyWatchTestObject creates an object and returns its revision.
func createKeyWatchTestObject(t *testing.T, ctx context.Context, ws world.WorldState, key string) uint64 {
	// Create the object with an example root block.
	t.Helper()
	obj, _, err := world.CreateWorldObject(ctx, ws, key, func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample("initial"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)

	// Read the new object's revision.
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}
