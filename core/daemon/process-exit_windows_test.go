package daemon

import (
	"testing"

	"golang.org/x/sys/windows"
)

// watchChildExit retains an event handle while the child is still alive.
func watchChildExit(t *testing.T, pid int) func() {
	// Open this fixture's handle before allowing it to exit.
	t.Helper()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })

	// Wait on the stable process handle without reaping exec.Cmd's child.
	return func() {
		t.Helper()
		if _, err := windows.WaitForSingleObject(handle, windows.INFINITE); err != nil {
			t.Fatal(err)
		}
	}
}
