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
// type, so they share one seqno; a watcher that waited for the next seqno
// never woke for the second, which hung the unixfs sync of a game files World.
func TestWaitRevInWriteTx(t *testing.T) {
	// Open an Engine on an empty World.
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
	bcs.SetBlock(world_block.NewWorld(false), true)
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
