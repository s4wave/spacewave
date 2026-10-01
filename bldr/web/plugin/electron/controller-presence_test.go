package electron

import (
	"context"
	"errors"
	"testing"

	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	"github.com/sirupsen/logrus"
)

// TestControllerPresenceRetainsShellFailure proves only process completion
// publishes ENDED and preserves its failure for both existing and late observers.
func TestControllerPresenceRetainsShellFailure(t *testing.T) {
	// Run a shell with a real desktop Resource acknowledgement and a controlled exit.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Build the controller with a runtime that fails on exit.
	r, err := NewController(logrus.NewEntry(logrus.New()), nil, "", "", "", "failure", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := newOpenRuntime(t, &openService{})
	exit := make(chan struct{})
	want := errors.New("Electron exited with status 1")

	// Attach the runtime and return the failure after the exit signal.
	r.run = func(ctx context.Context) error {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.runtime = rt
			broadcast()
		})
		select {
		case <-exit:
			return want
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Run the controller and register cleanup.
	done := make(chan error, 1)
	go func() { done <- r.Execute(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != context.Canceled {
			t.Errorf("controller exit = %v, want cancellation", err)
		}
	})

	// Capture the acknowledged shell before allowing its process to fail.

	// Open the shell and assert its presence reports active.
	generation, err := r.OpenOrFocusMainWindow(ctx, &bldr_web_plugin.OpenOrFocusDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	presence := r.DesktopPresence(generation)
	active := presence.GetValue()
	if active.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE {
		t.Fatalf("acknowledged presence = %v", active)
	}

	// Close the shell and assert the terminal presence retains the failure.
	close(exit)
	ended, err := presence.WaitValueChange(ctx, active, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ended.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
		t.Fatalf("failed shell presence = %v", ended)
	}
	if ended.GetError() != want.Error() {
		t.Fatalf("shell failure = %q, want %q", ended.GetError(), want)
	}

	// Read the terminal presence again as a late observer.
	// A late observer reads the same owner-confirmed terminal result.
	late := r.DesktopPresence(generation).GetValue()
	if !late.EqualVT(ended) {
		t.Fatalf("late presence = %v, want %v", late, ended)
	}
}
