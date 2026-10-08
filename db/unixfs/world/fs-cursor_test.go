package unixfs_world

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// failingWorldState fails every object lookup with err.
type failingWorldState struct {
	world.WorldState
	err error
}

// GetObject reports the lookup failure.
func (w failingWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	return nil, false, w.err
}

// TestGetProxyCursorReportsLookupError checks that a failed object lookup
// returns its own error, so a canceled lookup is not taken for a missing
// object.
func TestGetProxyCursorReportsLookupError(t *testing.T) {
	// Look up an object through a World that is canceled.
	ws := failingWorldState{err: context.Canceled}
	cursor := NewFSCursor(nil, ws, "test/lookup-error", FSType_FSType_FS_NODE, nil, false)
	t.Cleanup(cursor.Release)

	// The cursor reports the cancellation.
	if _, err := cursor.GetProxyCursor(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestNewFSCursorWithWriterConfirmsObservedRevision checks that a successful
// writer operation remains fenced on the cursor's observed object revision.
func TestNewFSCursorWithWriterConfirmsObservedRevision(t *testing.T) {
	// Construct a writer-backed FSCursor and verify its confirmation callback.
	cursor, writer := NewFSCursorWithWriterContext(
		context.Background(),
		nil,
		nil,
		"test/revision-confirmation",
		FSType_FSType_FS_NODE,
		"",
	)
	t.Cleanup(cursor.Release)
	confirm := writer.confirmFn.Load()
	if confirm == nil {
		t.Fatal("expected writer revision confirmation")
	}

	// A target newer than the observed revision must wait for the context.
	cursor.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		cursor.prevObjRev = 4
	})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (*confirm)(canceled, 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected confirmation to wait, got %v", err)
	}

	// An observed target must return even when the context is already canceled.
	cursor.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		cursor.prevObjRev = 5
	})
	if err := (*confirm)(canceled, 5); err != nil {
		t.Fatalf("expected observed revision confirmation, got %v", err)
	}
}
