package repofs

import (
	"context"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
)

// TestRepoCursorInvalidatesParentBeforeChildNotification checks snapshot invalidation order.
func TestRepoCursorInvalidatesParentBeforeChildNotification(t *testing.T) {
	// Open a repository projection and one cached child from its snapshot.
	ctx := context.Background()
	ws, obj, _ := newEngineTestState(t, ctx, "repo/parent-invalidation")
	t.Cleanup(func() { world.ReleaseObjectState(obj) })

	// Acquire the root before its descendant, matching filesystem traversal.
	root, err := OpenRepoFSCursor(ctx, ws, "repo/parent-invalidation", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Release)

	// Resolve a child that subscribes to the same repository revision watcher.
	ops, err := root.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := ops.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(child.Release)

	// A child notification must not allow reacquisition through its old parent.
	parentReleased := make(chan bool, 1)
	child.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		if ch.Released {
			parentReleased <- root.CheckReleased()
		}
		return false
	})

	// Commit through a separate engine, as another repository writer would.
	writer := NewEngine(ctx, ws, obj)
	t.Cleanup(writer.Close)
	tx, err := writer.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	ref := plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("main"),
		plumbing.NewHash("1111111111111111111111111111111111111111"),
	)
	if err := tx.SetReference(ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Observe parent state inside the child callback, before fan-out can finish.
	select {
	case released := <-parentReleased:
		if !released {
			t.Fatal("child notified while its parent still serves the old snapshot")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for repository invalidation")
	}
}
