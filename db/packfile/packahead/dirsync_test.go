package packahead

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestPackAheadDirSyncRetry fails the directory sync after a finalize's
// rename: the retry syncs the directory only, so the pack finalizes.
func TestPackAheadDirSyncRetry(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	var failed atomic.Bool
	dirSync = func(d string) error {
		names, _ := filepath.Glob(filepath.Join(d, "*"+packExt))
		if len(names) != 0 && failed.CompareAndSwap(false, true) {
			return syscall.EIO
		}
		return syncDir(d)
	}
	defer func() { dirSync = syncDir }()
	l, err := Open(ctx, dir, Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.Append(ctx, []Block{testBlock(t, 1, 3000)}); err != nil {
		t.Fatal(err)
	}
	waitFinalized(t, l, 1)
	if !failed.Load() {
		t.Fatal("the directory sync never failed")
	}
}

// TestPackAheadDirSyncRetryFailedPack fails the directory sync after the
// rename of a pack a write failed on: the retry does not recover the renamed
// file again, so the pack keeps its index and the next Open loads it.
func TestPackAheadDirSyncRetryFailedPack(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	var failed atomic.Bool
	dirSync = func(d string) error {
		names, _ := filepath.Glob(filepath.Join(d, "*"+packExt))
		if len(names) != 0 && failed.CompareAndSwap(false, true) {
			return syscall.EIO
		}
		return syncDir(d)
	}
	defer func() { dirSync = syncDir }()
	l, err := Open(ctx, dir, Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 0
	})

	// Batch a syncs; a write then fails on the open pack.
	a, b := testBlock(t, 1, 3000), testBlock(t, 2, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.failLocked(l.open, syscall.EIO)
	})
	release()

	// The next Append rotates; the maintainer recovers and finalizes the
	// failed pack, its directory sync fails once, and the retry finishes it.
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 10 * time.Millisecond
	})
	if err := l.Append(ctx, []Block{b}); err != nil {
		t.Fatal(err)
	}
	infos := waitFinalized(t, l, 2)
	if !failed.Load() {
		t.Fatal("the directory sync never failed")
	}
	for _, info := range infos {
		st, err := os.Stat(info.Path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() != info.Size {
			t.Fatalf("pack %d is %d bytes on disk, %d reported", info.Seq, st.Size(), info.Size)
		}
		checkPack(t, info, map[string][]byte{string(a.Key): a.Value, string(b.Key): b.Value}, Options{})
	}

	// The next Open loads both packs.
	l.Close()
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if !l2.HasDurable(a.Key) || !l2.HasDurable(b.Key) {
		t.Fatal("value lost after reopen")
	}
}
