//go:build !js

package checkoutroot

import (
	"bytes"
	"os"
	"runtime"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestDirectoryWatcherSharesRegistration keeps one native watch until its last release.
func TestDirectoryWatcherSharesRegistration(t *testing.T) {
	// Give two cursors the same directory.
	watcher, _ := openTestDirectoryWatcher(t, logrus.StandardLogger())
	leaveFirst := watcher.Watch(".", func(bool) {})
	leaveSecond := watcher.Watch(".", func(bool) {})
	if count := len(watcher.names); count != 1 {
		t.Fatalf("two cursors registered %d watches, want one", count)
	}

	// Retain the registration until both cursors have gone.
	leaveFirst()
	if count := len(watcher.names); count != 1 {
		t.Fatalf("one cursor retained %d watches, want one", count)
	}
	leaveSecond()
	if count := len(watcher.names); count != 0 {
		t.Fatalf("last cursor left %d watches, want none", count)
	}

	// Exercise registration failure and the Linux rename-before-dispatch race.
	t.Run("Degraded", checkDirectoryWatchDegraded)
	if runtime.GOOS == "linux" {
		t.Run("Renamed", checkDirectoryWatchRenamed)
	}
}

// checkDirectoryWatchDegraded keeps snapshots readable when registration fails.
func checkDirectoryWatchDegraded(t *testing.T) {
	// Capture the degradation warning from two failed subscriptions.
	var log bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&log)
	watcher, root := openTestDirectoryWatcher(t, logger)
	for range 2 {
		stop := watcher.Watch("missing", func(bool) { t.Error("snapshot received a watch event") })
		stop()
	}
	if count := bytes.Count(log.Bytes(), []byte("live directory refresh unavailable")); count != 1 {
		t.Fatalf("received %d degradation warnings, want one", count)
	}

	// A missing watch leaves the root readable and other paths watchable.
	stop := watcher.Watch(".", func(bool) {})
	defer stop()
	if _, err := root.Stat("."); err != nil {
		t.Fatal(err)
	}
	if count := len(watcher.names); count != 1 {
		t.Fatalf("healthy directory registered %d watches, want one", count)
	}
}

// checkDirectoryWatchRenamed proves stale release preserves live settled-write events.
func checkDirectoryWatchRenamed(t *testing.T) {
	// Prepare a directory with a file and a separate event barrier.
	watcher, root := openTestDirectoryWatcher(t, logrus.StandardLogger())
	for _, name := range []string{"old", "barrier"} {
		if err := root.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := root.WriteFile("old/file", nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Open the new name before dispatch processes the queued rename.
	events := make(chan string, 16)
	leaveOld := watcher.Watch("old", func(bool) { events <- "old" })
	if err := root.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	leaveNew := watcher.Watch("new", func(removed bool) {
		if removed {
			events <- "removed"
			return
		}
		events <- "new"
	})
	defer leaveNew()
	if count := len(watcher.names); count != 1 {
		t.Fatalf("renamed inode registered %d watches, want one", count)
	}
	leaveOld()

	// Queue a permission change and several writes through the same descriptor.
	leaveBarrier := watcher.Watch("barrier", func(bool) { events <- "barrier" })
	defer leaveBarrier()
	if err := root.Chmod("new/file", 0o640); err != nil {
		t.Fatal(err)
	}
	file, err := root.OpenFile("new/file", os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	for range 3 {
		if _, err := file.Write([]byte("more")); err != nil {
			t.Fatal(err)
		}
	}

	// Separate events from the open write and its close with queued directory changes.
	if err := root.Mkdir("barrier/open", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir("barrier/closed", 0o700); err != nil {
		t.Fatal(err)
	}

	// The first wake rearms the moved inode and the second follows the single close.
	done := make(chan struct{})
	go func() {
		defer close(done)
		watcher.Run(t.Context())
	}()
	t.Cleanup(func() { <-done })
	for _, want := range []string{"new", "barrier", "new", "barrier"} {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("received %q, want %q", got, want)
			}
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
}

// openTestDirectoryWatcher opens a temporary root and verifies native refresh.
func openTestDirectoryWatcher(t *testing.T, logger *logrus.Logger) (*DirectoryWatcher, *os.Root) {
	// Release the native source before closing the root.
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	watcher := NewDirectoryWatcher(logrus.NewEntry(logger), root)
	t.Cleanup(func() { _ = watcher.Close() })
	if watcher.native == nil {
		t.Fatal("could not open the native watcher")
	}
	return watcher, root
}
