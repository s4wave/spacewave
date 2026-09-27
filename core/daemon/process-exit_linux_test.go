package daemon

import (
	"testing"

	"golang.org/x/sys/unix"
)

// watchChildExit waits for the owned child's exit without consuming its status.
func watchChildExit(t *testing.T, pid int) func() {
	return func() {
		t.Helper()
		var info unix.Siginfo
		if err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil); err != nil {
			t.Fatal(err)
		}
	}
}
