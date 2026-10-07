package world_block

import (
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestReadChangeSet checks the changes read after a base seqno, and that a
// base no longer in the history yields an unknown ChangeSet.
func TestReadChangeSet(t *testing.T) {
	// Create two objects, one entry each.
	ctx := t.Context()
	ws, cursor, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	ref := writeTestBlock(t, ctx, cursor, "body")
	create := func(key string) {
		// Create and commit one object.
		t.Helper()
		obj, err := ws.CreateObject(ctx, key, ref)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if err := ws.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	create("a")
	create("b")

	// The changes after the first entry name only the second key.
	seqno, head, changes, err := readChangeSet(ctx, ws.bcs, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if seqno != 2 || head.Seqno != 2 || changes.Unknown || !slices.Equal(changes.Keys, []string{"b"}) {
		t.Fatalf("read seqno %d head %d changes %+v", seqno, head.Seqno, changes)
	}

	// Continuing from the reported head names only the next key.
	create("c")
	seqno, _, changes, err = readChangeSet(ctx, ws.bcs, seqno, head)
	if err != nil {
		t.Fatal(err)
	}
	if seqno != 3 || changes.Unknown || !slices.Equal(changes.Keys, []string{"c"}) {
		t.Fatalf("read seqno %d changes %+v", seqno, changes)
	}

	// A base that left the history yields an unknown ChangeSet.
	for i := range 2 * ChangeLogSegmentLen {
		create("obj-" + strconv.Itoa(i))
	}
	_, _, changes, err = readChangeSet(ctx, ws.bcs, 2, head)
	if err != nil {
		t.Fatal(err)
	}
	if !changes.Unknown {
		t.Fatalf("read precise changes %+v from a dropped base", changes)
	}
}
