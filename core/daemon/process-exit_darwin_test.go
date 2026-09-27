package daemon

import (
	"testing"

	"golang.org/x/sys/unix"
)

// watchChildExit subscribes before the child exits and waits without reaping it.
func watchChildExit(t *testing.T, pid int) func() {
	// Register the process event while the fixture is still waiting for input.
	t.Helper()
	kq, err := unix.Kqueue()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(kq) })
	changes := []unix.Kevent_t{{
		Ident: uint64(pid), Filter: unix.EVFILT_PROC,
		Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT,
	}}
	if _, err := unix.Kevent(kq, changes, nil, nil); err != nil {
		t.Fatal(err)
	}

	// NOTE_EXIT observes termination while leaving the zombie's PID reserved.
	return func() {
		t.Helper()
		events := make([]unix.Kevent_t, 1)
		if _, err := unix.Kevent(kq, nil, events, nil); err != nil {
			t.Fatal(err)
		}
		if events[0].Fflags&unix.NOTE_EXIT == 0 {
			t.Fatalf("unexpected process event: %+v", events[0])
		}
	}
}
