package world_block

import (
	"context"
	"testing"
	"time"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestReadSnapshotAccessWithoutEngineLock reads a read snapshot's storage
// while another goroutine holds the Engine lock: the snapshot follows refs
// from the head it holds, so a reader never queues on the lock per object.
// Discard releases the held head's pin.
func TestReadSnapshotAccessWithoutEngineLock(t *testing.T) {
	// Open an engine on an empty World.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Start the engine on an empty cursor.
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()
	eng, err := NewEngine(ctx, le, base, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Open a read snapshot. The snapshot holds the published head for its
	// object reads and for its state.
	head := eng.pub.Load()
	before := head.refs.Load()
	rtx, err := eng.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := head.refs.Load(); got != before+2 {
		t.Fatalf("snapshot holds %d head references, want %d", got, before+2)
	}

	// Follow the snapshot's root while the Engine lock is held.
	locked := eng.bcast.Lock()
	done := make(chan error, 1)
	go func() {
		done <- rtx.AccessWorldState(ctx, rtx.readRoot, func(*bucket_lookup.Cursor) error { return nil })
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		locked.Unlock()
		<-done
		t.Fatal("a read snapshot waited on the Engine lock to read its storage")
	}
	locked.Unlock()

	// Discard releases the held head, and reads after Discard fall back to
	// the Engine.
	rtx.Discard()
	if got := head.refs.Load(); got != before {
		t.Fatalf("discarded snapshot left %d head references, want %d", got, before)
	}
}
