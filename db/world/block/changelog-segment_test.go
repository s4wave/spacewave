package world_block

import (
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestChangeLogSegmentsBoundHistory checks that the changelog retains the
// current and previous segments, contiguous from the head.
func TestChangeLogSegmentsBoundHistory(t *testing.T) {
	// Alternate object creation and deletion so each change is its own entry.
	ctx := t.Context()
	ws, cursor, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	ref := writeTestBlock(t, ctx, cursor, "body")
	for i := range ChangeLogSegmentLen * 3 / 2 {
		key := "obj-" + strconv.Itoa(i)
		obj, err := ws.CreateObject(ctx, key, ref)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.DeleteObject(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The history starts at the first entry of the previous segment.
	root, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := root.GetLastChange().GetSeqno()
	if head <= 2*ChangeLogSegmentLen {
		t.Fatalf("head seqno %d does not span three segments", head)
	}
	oldest := (head-1)/ChangeLogSegmentLen*ChangeLogSegmentLen + 1 - ChangeLogSegmentLen

	// Read the whole history and require every seqno from the head down.
	entries, err := ReadChangeLogEntriesFromCursor(ctx, ws.bcs, ChangeLogReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := uint64(len(entries)), head-oldest+1; got != want {
		t.Fatalf("read %d entries, want %d", got, want)
	}
	for i, entry := range entries {
		if entry.Seqno != head-uint64(i) || len(entry.Changes) == 0 {
			t.Fatalf("entry %d has seqno %d and %d changes", i, entry.Seqno, len(entry.Changes))
		}
	}

	// A cursor inside the previous segment reads only the newer entries.
	after := oldest + 10
	entries, err = ReadChangeLogEntriesFromCursor(ctx, ws.bcs, ChangeLogReadOptions{AfterSeqno: after})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := uint64(len(entries)), head-after; got != want {
		t.Fatalf("read %d entries after %d, want %d", got, after, want)
	}
}
