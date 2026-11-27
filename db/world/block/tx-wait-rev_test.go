package world_block_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestWaitRevInWriteTx checks that a watcher inside a write transaction wakes
// on each revision of its object. Two revisions in a row are changes of one
// type, so they share one seqno. A watcher that waited for the next seqno
// never woke for the second, which hung the unixfs sync of a game files World.
func TestWaitRevInWriteTx(t *testing.T) {
	ctx := t.Context()
	engine := newWaitRevEngine(t)

	// Create the object in a write transaction and read its revision.
	tx, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Watch from a second handle, as the unixfs cursor does, and bump the
	// revision once per wake.
	watcher, found, err := tx.GetObject(ctx, "a")
	if err != nil || !found {
		t.Fatal(found, err)
	}
	defer world.ReleaseObjectState(watcher)
	for want := rev + 1; want <= rev+3; want++ {
		done := make(chan error, 1)
		go func() {
			_, err := watcher.WaitRev(ctx, want, false)
			done <- err
		}()
		// Let the watcher start waiting before the revision moves.
		time.Sleep(20 * time.Millisecond)
		if _, err := obj.IncrementRev(ctx); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("rev %d: %v", want, err)
			}
		case <-waitCtx.Done():
			t.Fatalf("WaitRev(%d) did not wake", want)
		}
		cancel()
	}
}

// newWaitRevEngine opens an Engine on an empty World.
func newWaitRevEngine(t *testing.T) *world_block.Engine {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)
	btx, bcs := ocs.BuildTransaction(nil)
	bcs.SetBlock(world_block.NewWorld(world_block.NewestWorldFormat, false), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ocs.SetRootRef(rootRef)
	engine, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

// TestWaitObjectRevCreateInWriteTx checks that a wait for a missing object
// inside a write transaction wakes when the object is created. The create
// follows a create, so the two share one seqno. A wait for the next seqno
// never woke.
func TestWaitObjectRevCreateInWriteTx(t *testing.T) {
	ctx := t.Context()
	engine := newWaitRevEngine(t)
	tx, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	a, err := tx.CreateObject(ctx, "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(a)
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for "b" through the generic helper, as a guarded handle does.
	done := make(chan error, 1)
	go func() {
		_, err := world.WaitObjectRev(ctx, tx, "b", 0, true)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	b, err := tx.CreateObject(ctx, "b", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(b)

	// The create left the seqno where it was, so only the key wake ends the
	// wait.
	if next, err := tx.GetSeqno(ctx); err != nil || next != seqno {
		t.Fatalf("seqno after second create: %d, %v; want %d", next, err, seqno)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitObjectRev for a created object did not wake")
	}
}

// TestWaitRevWakesOnlyItsObject checks that a change to one object wakes only
// that object's waiters.
func TestWaitRevWakesOnlyItsObject(t *testing.T) {
	ctx := t.Context()
	engine := newWaitRevEngine(t)
	tx, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	a, err := tx.CreateObject(ctx, "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(a)
	b, err := tx.CreateObject(ctx, "b", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(b)

	// A change to "b" leaves the wake of "a" open.
	woken := world_block.ObjectRevWake(tx, "a")
	if _, err := b.IncrementRev(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-woken:
		t.Fatal("a change to b woke the waiters of a")
	default:
	}

	// A change to "a" wakes them.
	if _, err := a.IncrementRev(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-woken:
	default:
		t.Fatal("a change to a did not wake its waiters")
	}
}
