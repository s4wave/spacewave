package web_pkg

import (
	"context"
	"testing"
)

func TestRoutineGroupRejectsAfterStopAccepting(t *testing.T) {
	// Close and drain the routine group before submitting more work.
	var group RoutineGroup
	group.StopAccepting()
	group.Wait()

	// Wrap a routine that records whether the group lets it start.
	started := false
	routine := group.Wrap(func(context.Context) error {
		started = true
		return nil
	})

	// Verify the closed routine group rejects the routine before it starts.
	if err := routine(context.Background()); err != context.Canceled {
		t.Fatalf("routine after StopAccepting: got %v, want %v", err, context.Canceled)
	}
	if started {
		t.Fatal("routine started after StopAccepting")
	}
}
