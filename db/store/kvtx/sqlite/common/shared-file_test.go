//go:build linux || darwin

package common_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// sharedLockFirst and sharedLockSize are the bytes of the database file
// SQLite's unix VFS read-locks for a connection's SHARED lock, which a
// connection in WAL mode holds while it is open.
const (
	sharedLockFirst = 0x40000002
	sharedLockSize  = 510
)

// TestSharedFileLocks opens two Stores on one file, as the volume coordinator
// does for two tables, writes the file back through the first's checkpointer,
// and closes it. Closing a descriptor drops every POSIX lock the process holds
// on the file, so a close of the first Store's descriptor would leave the
// second's connections unlocked while they still read. Another process must
// still be refused the write lock that conflicts with the second's SHARED
// lock, and still once the second Store closes under a read it has open,
// whose connection stays open; it is granted the lock once that read ends.
// OFD locks belong to the descriptor that took them and survive any other
// close, so under a driver that takes them the test proves nothing.
func TestSharedFileLocks(t *testing.T) {
	// A child process reports whether it can take the conflicting lock.
	if path := os.Getenv("SPACEWAVE_SHARED_FILE_PROBE"); path != "" {
		probeSharedLock(path)
		return
	}

	// Write through the first Store, with a reader of the second pinning the
	// log so it passes the write-back size, until the first's checkpointer
	// writes the database file back.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared.db")
	written := make(chan struct{}, 1)
	common.SetCheckpointHook(func(step string) {
		if step == "written back" {
			select {
			case written <- struct{}{}:
			default:
			}
		}
	}, 25*time.Millisecond)
	defer common.SetCheckpointHook(nil, 25*time.Millisecond)
	first := openRestartStore(t, path)
	drv := reflect.TypeOf(first.GetDB().Driver())
	if drv.Kind() == reflect.Pointer {
		drv = drv.Elem()
	}
	if pkg := drv.PkgPath(); strings.Contains(pkg, "ncruces") {
		_ = first.Close()
		t.Skip("the driver takes OFD locks: " + pkg)
	}
	second, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, "kv2", store_kvtx_sqlite.Pragmas{WALCheckpointBytes: restartBound})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := second.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pin.Get(ctx, restartKey(0)); err != nil {
		t.Fatal(err)
	}
	writing, stopWriting := context.WithCancel(ctx)
	defer stopWriting()
	writeDone := make(chan error, 1)
	go func() {
		var committed atomic.Int64
		writeDone <- writeUntilClosed(writing, first, path, &committed)
	}()
	select {
	case <-written:
	case err := <-writeDone:
		t.Fatalf("writer stopped before a write-back: %v", err)
	}
	stopWriting()
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	pin.Discard()

	// Close the first Store and read through the second, whose connections
	// keep their SHARED lock.
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	tx, err := second.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tx.Get(ctx, restartKey(0)); err != nil {
		t.Fatal(err)
	}
	if got := runSharedLockProbe(t, path); got != "refused" {
		t.Fatalf("with the second Store open another process was %s the write lock", got)
	}

	// Close the second Store under its read, whose connection keeps its lock
	// until the read ends.
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if got := runSharedLockProbe(t, path); got != "refused" {
		t.Fatalf("with a read open past its Store's close another process was %s the write lock", got)
	}
	tx.Discard()
	if got := runSharedLockProbe(t, path); got != "locked" {
		t.Fatalf("with both Stores closed another process was %s the write lock", got)
	}
}

// runSharedLockProbe runs the test binary as a probe of the database file at
// path and returns its answer: "locked" or "refused".
func runSharedLockProbe(t *testing.T, path string) string {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSharedFileLocks$")
	cmd.Env = append(os.Environ(), "SPACEWAVE_SHARED_FILE_PROBE="+path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe: %v: %s", err, out)
	}
	for line := range strings.Lines(string(out)) {
		if answer, ok := strings.CutPrefix(strings.TrimSpace(line), "probe "); ok {
			return answer
		}
	}
	t.Fatalf("probe gave no answer: %s", out)
	return ""
}

// probeSharedLock tries once for a write lock on the SHARED lock bytes of the
// database file at path and prints whether it took it.
func probeSharedLock(path string) {
	// Open the file and try the lock without waiting.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		os.Stdout.WriteString("probe open failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer f.Close()
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: sharedLockFirst, Len: sharedLockSize}
	if err := unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lock); err != nil {
		os.Stdout.WriteString("probe refused\n")
		return
	}
	os.Stdout.WriteString("probe locked\n")
}
