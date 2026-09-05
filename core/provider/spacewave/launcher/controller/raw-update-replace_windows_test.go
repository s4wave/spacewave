package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestReplaceFileWaitsForReader checks replacement after a transient Windows handle closes.
func TestReplaceFileWaitsForReader(t *testing.T) {
	// Retain the installed file through a reader that denies delete sharing.
	dir := t.TempDir()
	target := filepath.Join(dir, "installed.exe")
	source := filepath.Join(dir, "replacement.exe")
	if err := os.WriteFile(target, []byte("installed version"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement version"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if handle != windows.InvalidHandle {
			_ = windows.CloseHandle(handle)
		}
	})

	// A transient lock must not terminate the replacement before it clears.
	done := make(chan error, 1)
	go func() { done <- replaceFile(source, target) }()
	select {
	case err := <-done:
		t.Fatalf("replacement returned while the destination was locked: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = windows.InvalidHandle
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not resume after the reader closed")
	}

	// Accept the replacement only when its actual bytes occupy the installed path.
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "replacement version" {
		t.Fatalf("installed bytes: %q", data)
	}
}
