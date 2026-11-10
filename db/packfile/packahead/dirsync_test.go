package packahead

import (
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
