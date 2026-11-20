//go:build !js

package checkoutroot

import (
	"os"
	"testing"
)

// TestDirectoryWatcherSharesRegistration keeps one native watch until its last release.
func TestDirectoryWatcherSharesRegistration(t *testing.T) {
	// Open a root and subscribe two cursors to the same directory.
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	watcher, err := NewDirectoryWatcher(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watcher.Close() })

	// Check the native source rather than the watcher's subscriber bookkeeping.
	leaveFirst, err := watcher.Watch(".", func(bool) {})
	if err != nil {
		t.Fatal(err)
	}
	leaveSecond, err := watcher.Watch(".", func(bool) {})
	if err != nil {
		t.Fatal(err)
	}
	if paths := watcher.native.WatchList(); len(paths) != 1 {
		t.Fatalf("two cursors created %v, want exactly one native watch", paths)
	}

	// Releasing either cursor keeps the watch until the last subscriber leaves.
	leaveFirst()
	if paths := watcher.native.WatchList(); len(paths) != 1 {
		t.Fatalf("one remaining cursor has watches %v, want one", paths)
	}
	leaveSecond()
	if paths := watcher.native.WatchList(); len(paths) != 0 {
		t.Fatalf("last cursor left native watches %v", paths)
	}
}
