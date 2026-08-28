package unixfs_world

import (
	"context"
	"errors"
	"testing"
)

// TestNewFSCursorWithWriterConfirmsObservedRevision checks that a successful
// writer operation remains fenced on the cursor's observed object revision.
func TestNewFSCursorWithWriterConfirmsObservedRevision(t *testing.T) {
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
