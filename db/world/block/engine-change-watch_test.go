package world_block

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestEngineChangeWatchWakesOnlyChangedKeys checks that a commit wakes the
// waiters of the objects it changed and leaves other waiters asleep, also when
// the commit changes too many objects to list.
func TestEngineChangeWatchWakesOnlyChangedKeys(t *testing.T) {
	for _, bulk := range []int{0, changeDiffLimit} {
		t.Run(fmt.Sprintf("bulk-%d", bulk), func(t *testing.T) {
			testChangeWatchWakesOnlyChangedKeys(t, bulk)
		})
	}
}

// testChangeWatchWakesOnlyChangedKeys runs the wake check with a commit that
// also creates bulk other objects.
func testChangeWatchWakesOnlyChangedKeys(t *testing.T, bulk int) {
	// Create two objects on a real block Engine.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "watch/quiet")
	createWatchTestObject(t, ctx, ws, "watch/busy")

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
	quietCh, busyCh := quietWake.state.Load().ch, busyWake.state.Load().ch
	locked.Unlock()
	defer e.unwatchObjectKey(quietWatch, "watch/quiet", quietWake)
	defer e.unwatchObjectKey(busyWatch, "watch/busy", busyWake)

	// Change the busy object in one commit with the bulk objects.
	tx, err := e.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for i := range bulk {
		obj, err := tx.CreateObject(ctx, fmt.Sprintf("watch/bulk/%05d", i), nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := tx.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("watch/busy", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// One comparison wakes every changed key under one lock, so the busy
	// waiter's wakeup proves the quiet key was compared and left unchanged.
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
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	rev := createWatchTestObject(t, ctx, ws, "watch/object")

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

// TestEngineWaitersShareTheHead checks that a head hands its seqno to the
// seqno waiters, and a changed object's revision to all of its waiters, so a
// head wakes them without a lock or a read each. It fails when a waiter takes
// the engine lock to read the seqno, or when the change watch leaves each
// object waiter to read the object itself.
func TestEngineWaitersShareTheHead(t *testing.T) {
	// Create the watched object on a real block Engine.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	rev := createWatchTestObject(t, ctx, ws, "watch/shared")

	// A seqno wait returns while the engine lock is held.
	seqno, err := e.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	locked := e.bcast.Lock()
	done := make(chan error, 1)
	go func() {
		got, err := e.WaitSeqno(ctx, seqno)
		if err == nil && got != seqno {
			err = errors.Errorf("WaitSeqno = %d, want %d", got, seqno)
		}
		done <- err
	}()
	select {
	case err := <-done:
		locked.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		locked.Unlock()
		t.Fatal("a satisfied seqno wait took the engine lock")
	}

	// Many waiters on one object all return its next revision.
	const waiters = 64
	results := make(chan error, waiters)
	for range waiters {
		go func() {
			got, err := e.WaitObjectRev(ctx, "watch/shared", rev+1, false)
			if err == nil && got != rev+1 {
				err = errors.Errorf("WaitObjectRev = %d, want %d", got, rev+1)
			}
			results <- err
		}()
	}
	var wake *objectKeyWake
	for refs := 0; refs != waiters; {
		locked := e.bcast.Lock()
		if w := e.changeWatch; w != nil {
			wake = w.waiters["watch/shared"]
		}
		if wake != nil {
			refs = wake.refs
		}
		locked.Unlock()
		if refs != waiters {
			time.Sleep(time.Millisecond)
		}
	}
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("watch/shared", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	for range waiters {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	// The wake that ended them carried the revision the watch read once.
	st := wake.state.Load()
	locked = e.bcast.Lock()
	head := e.published
	locked.Unlock()
	if !st.known || !st.found || st.rev != rev+1 || st.head != head {
		t.Fatalf("the key wake holds known %v, found %v, rev %d at head %d; want rev %d at head %d", st.known, st.found, st.rev, st.head, rev+1, head)
	}
}

// TestEngineWaitObjectRevIgnoresAnOlderHead checks that an object waiter
// reads the object itself when a wake carries a revision from a head older
// than the one it read, since a head's seqno can repeat or move back. It
// fails when a waiter takes any revision a wake carries.
func TestEngineWaitObjectRevIgnoresAnOlderHead(t *testing.T) {
	// Wait for the next revision of an object.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	rev := createWatchTestObject(t, ctx, ws, "watch/older")
	done := make(chan error, 1)
	go func() {
		_, err := e.WaitObjectRev(ctx, "watch/older", rev+1, false)
		done <- err
	}()
	var wake *objectKeyWake
	for wake == nil {
		locked := e.bcast.Lock()
		if w := e.changeWatch; w != nil {
			wake = w.waiters["watch/older"]
		}
		locked.Unlock()
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	// A wake from before the waiter's head does not satisfy it.
	locked := e.bcast.Lock()
	wake.publish(&objectKeyState{known: true, head: e.published - 1, rev: rev + 1, found: true})
	locked.Unlock()
	select {
	case err := <-done:
		t.Fatalf("a wake from an older head ended the wait: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// The object's next revision does.
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("watch/older", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// createWatchTestObject creates an object and returns its revision.
func createWatchTestObject(t *testing.T, ctx context.Context, ws world.WorldState, key string) uint64 {
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
