package packahead

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

// TestPackAheadCloseFinalizes closes a Log cleanly: the open pack is
// finalized through the normal path, so the next Open finds no unfinalized
// pack and copies nothing, and the empty spare is removed.
func TestPackAheadCloseFinalizes(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	l, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := testBlock(t, 1, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	if err := l.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}

	// Only the finalized pack is left.
	opens, _ := filepath.Glob(filepath.Join(dir, "*"+openExt))
	packs, _ := filepath.Glob(filepath.Join(dir, "*.pack"))
	if len(opens) != 0 || len(packs) != 1 {
		t.Fatalf("after close: open %v, packs %v", opens, packs)
	}
	before, err := os.Stat(packs[0])
	if err != nil {
		t.Fatal(err)
	}

	// The next Open loads it as it is.
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	infos, _ := l2.GetPacks()
	if len(infos) != 1 {
		t.Fatalf("packs after open: %+v", infos)
	}
	after, err := os.Stat(infos[0].Path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("Open copied a pack Close finalized: %v", err)
	}
	if st := l2.GetStats(); st.Salvaged != 0 {
		t.Fatalf("stats: %+v", st)
	}
	if got, ok, err := l2.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("value after open: %v %v", ok, err)
	}
}

// TestPackAheadCloseEmpty closes a Log that took no batch: its empty open
// pack and spare are removed, not finalized.
func TestPackAheadCloseEmpty(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	l, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("files after an empty close: %v", names)
	}
}

// TestPackAheadCloseFinalizeFails fails the rename of Close's finalize: the
// pack stays unfinalized, and the next Open salvages it with its value.
func TestPackAheadCloseFinalizeFails(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	l, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := testBlock(t, 1, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	var fail atomic.Bool
	fail.Store(true)
	setHook(t, &renameFile, func(from, to string) error {
		if fail.Load() {
			return syscall.EIO
		}
		return os.Rename(from, to)
	})
	if err := l.CloseContext(ctx); err == nil {
		t.Fatal("close finalized through a failed rename")
	}
	fail.Store(false)
	if _, err := os.Stat(filepath.Join(dir, openName(1))); err != nil {
		t.Fatalf("unfinalized pack after a failed close: %v", err)
	}

	// The next Open salvages it.
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if got, ok, err := l2.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("value after open: %v %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); !os.IsNotExist(err) {
		t.Fatalf("unfinalized pack after open: %v", err)
	}
}
