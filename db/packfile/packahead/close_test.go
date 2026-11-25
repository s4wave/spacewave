package packahead

import (
	"os"
	"testing"
	"time"
)

// TestCloseWaitsSync closes the Log while a writer runs a sync: Close must
// not close the pack file under the sync, so it returns only after the sync
// does, and the sync still finds its file open.
func TestCloseWaitsSync(t *testing.T) {
	ctx := t.Context()
	entered, release := make(chan struct{}), make(chan struct{})
	statErr := make(chan error, 1)
	first := true
	hook := func(f *os.File) error {
		if !first {
			return f.Sync()
		}
		first = false
		close(entered)
		<-release
		_, err := f.Stat()
		statErr <- err
		return err
	}
	l, err := Open(ctx, t.TempDir(), Options{SyncData: hook})
	if err != nil {
		t.Fatal(err)
	}

	// Hold a writer in its sync.
	appended := make(chan error, 1)
	go func() { appended <- l.Append(ctx, []Block{testBlock(t, 1, 3000)}) }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("append never synced")
	}

	// Close waits for the sync.
	closed := make(chan struct{})
	go func() {
		l.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-statErr; err != nil {
		t.Fatalf("sync ran on a closed file: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("close never returned")
	}
	if err := <-appended; err != nil {
		t.Fatalf("append: %v", err)
	}
}
