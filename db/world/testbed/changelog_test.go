package testbed

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// errStopWatch ends WatchChanges after the first report.
var errStopWatch = errors.New("stop watch")

// TestChangelogWatchChanges checks that a World with the changelog enabled
// commits a batch of object and graph writes and reports them through
// WatchChanges.
func TestChangelogWatchChanges(t *testing.T) {
	// Start a World with the changelog and read its seqno.
	ctx := t.Context()
	tb := MustDefault(t, ctx, WithChangelog())
	seqno, err := tb.Engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open a write transaction.
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Create more objects and quads than fit inline in one changelog entry.
	keys := make([]string, 12)
	for i := range keys {
		keys[i] = "obj-" + strconv.Itoa(i)
		obj, err := tx.CreateObject(ctx, keys[i], nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if i != 0 {
			if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(keys[0], "<link>", keys[i], "")); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Commit the batch.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The feed reports every key and quad.
	var changes *world.ChangeSet
	err = world.WatchChanges(ctx, tb.Engine, seqno, func(_ uint64, cs *world.ChangeSet) error {
		changes = cs
		return errStopWatch
	})
	if !errors.Is(err, errStopWatch) {
		t.Fatal(err)
	}
	if changes.Unknown || !slices.Equal(slices.Sorted(slices.Values(changes.Keys)), slices.Sorted(slices.Values(keys))) || len(changes.Quads) != len(keys)-1 {
		t.Fatalf("changes = %+v", changes)
	}
}
