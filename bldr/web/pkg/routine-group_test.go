package web_pkg

import (
	"context"
	"testing"
)

func TestRoutineGroupRejectsAfterStopAccepting(t *testing.T) {
	var group RoutineGroup
	group.StopAccepting()
	group.Wait()

	started := false
	routine := group.Wrap(func(context.Context) error {
		started = true
		return nil
	})
	if err := routine(context.Background()); err != context.Canceled {
		t.Fatalf("routine after StopAccepting: got %v, want %v", err, context.Canceled)
	}
	if started {
		t.Fatal("routine started after StopAccepting")
	}
}
