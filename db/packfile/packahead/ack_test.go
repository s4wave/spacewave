package packahead

import (
	"bytes"
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
)

// setHook replaces the test hook at v with h until the test ends.
func setHook[T any](t *testing.T, v *T, h T) {
	old := *v
	*v = h
	t.Cleanup(func() { *v = old })
}

// requireReopened closes l, opens dir again, and checks every block in acked
// reads back with its value.
func requireReopened(t *testing.T, l *Log, dir string, acked []Block) {
	t.Helper()
	l.Close()
	l2, err := Open(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer l2.Close()
	for _, b := range acked {
		if got, ok, err := l2.Get(b.Key); err != nil || !ok || !bytes.Equal(got, b.Value) {
			t.Fatalf("acked block %x lost after reopen: ok=%v err=%v", b.Key[:4], ok, err)
		}
	}
}

// TestPackAheadWriteFailsDuringSync fails a second writer's write while the
// first writer leads the sync covering its own batch. The failed write drops
// the batches past the synced mark from the table and the pack is salvaged
// from that mark, so the leader must not publish its sync: the first
// writer's Append returns an error, and an Append that returned nil must find
// its value after a reopen.
func TestPackAheadWriteFailsDuringSync(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// Hold the first data sync open, and fail batch writes while armed.
	entered, release := make(chan struct{}), make(chan struct{})
	var holdSync, failWrite atomic.Bool
	syncHook := func(f *os.File) error {
		if holdSync.CompareAndSwap(true, false) {
			close(entered)
			<-release
		}
		return f.Sync()
	}
	setHook(t, &batchWriteAt, func(f *os.File, b []byte, off int64) (int, error) {
		if failWrite.Load() {
			return 0, syscall.EIO
		}
		return f.WriteAt(b, off)
	})
	l, err := Open(ctx, dir, Options{SyncData: syncHook})
	if err != nil {
		t.Fatal(err)
	}

	// Writer A leads the sync of its batch, and writer B's write fails under it.
	a, b := testBlock(t, 1, 3000), testBlock(t, 2, 3000)
	holdSync.Store(true)
	aErr := make(chan error, 1)
	go func() { aErr <- l.Append(ctx, []Block{a}) }()
	<-entered
	failWrite.Store(true)
	if err := l.Append(ctx, []Block{b}); err == nil {
		t.Fatal("append succeeded on a failed write")
	}
	failWrite.Store(false)
	close(release)

	// A's batch was dropped, so its Append fails rather than acking a value
	// no pack keeps.
	var acked []Block
	if err := <-aErr; err == nil {
		acked = append(acked, a)
	}
	requireReopened(t, l, dir, acked)
	if len(acked) != 0 {
		t.Fatal("append acked a batch its pack's salvage dropped")
	}
}
