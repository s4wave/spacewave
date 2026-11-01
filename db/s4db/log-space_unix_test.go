//go:build darwin || linux

package s4db

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sys/unix"
)

// logLimitEnv selects the isolated process whose file growth is capped.
const logLimitEnv = "S4DB_TEST_LOG_LIMIT"

// TestLogReserve uses real file IO with a kernel growth limit. A regular record
// must fail before consuming the reserve; cleanup and recovery use backed bytes.
func TestLogReserve(t *testing.T) {
	// Isolate the resource limit and SIGXFSZ disposition from every other test.
	path := os.Getenv(logLimitEnv)
	if path == "" {
		dir := t.TempDir()
		if root := os.Getenv("S4DB_FULL_DISK_DIR"); root != "" {
			var err error
			dir, err = os.MkdirTemp(root, "s4db-reserve-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
		}
		path = filepath.Join(dir, "reserve.s4wave")
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLogReserve$", "-test.timeout=60s", "-test.v")
		cmd.Env = append(os.Environ(), logLimitEnv+"="+path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		} else {
			t.Logf("%s", out)
		}
		return
	}

	// Preserve a normal acknowledged value and an unused job reservation.
	ctx := t.Context()
	db := openTest(t, path, Options{})
	db.sched.stop()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("kept"), []byte("acknowledged")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Leave a real job reservation for the full-disk cleanup.
	reserved, err := db.w.reserve(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Fill the active extent with valid bounded-key transactions. Adjust the
	// last record so a link fits but the next free record must roll over.
	base := db.cur.Load()
	target := uint64(recordHeader + (&record{kind: kindFree, chunks: reserved}).bound() + linkSize - 1)
	for base.end-base.pos > target+MaxKeySize {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, bytes.Repeat([]byte("k"), MaxKeySize/2), []byte("v")); err != nil {
			t.Fatal(err)
		}
		if err := kvtx.CommitOrdered(ctx, tx); err != nil {
			t.Fatal(err)
		}
		base = db.cur.Load()
	}

	// Size the final legal key using the actual record encoding.
	want := int(base.end - base.pos - target)
	filler := &record{
		kind: kindCommit, delta: 1, hot: base.m.hot, cold: base.m.cold,
		ops: []op{{key: bytes.Repeat([]byte("k"), want-recordHeader-32), val: value{kind: valueInline, inline: []byte("v")}}},
	}
	for {
		delta := want - len(filler.encode(0))
		if delta == 0 {
			break
		}
		filler.ops[0].key = bytes.Repeat([]byte("k"), len(filler.ops[0].key)+delta)
	}

	// Publish the prepared key through the normal transaction interface.
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, filler.ops[0].key, []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if st := db.cur.Load(); st.end != base.end || st.end-st.pos != target {
		t.Fatalf("did not reach the log boundary: remaining=%d want=%d", st.end-st.pos, target)
	}

	// The OS now refuses growth, while existing active and standby bytes remain
	// writable. No fake allocator or production test seam supplies the result.
	var old unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &old); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Apply a process-local growth cap or exhaust the private filesystem.
	signal.Ignore(unix.SIGXFSZ)
	limit := old
	limit.Cur = uint64(info.Size())
	if os.Getenv("S4DB_FULL_DISK_DIR") != "" {
		fillLogDisk(t, path)
	} else if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Setrlimit(unix.RLIMIT_FSIZE, &old) }()

	// An ordinary record must fail before consuming its guaranteed reserve.
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("unacknowledged"), bytes.Repeat([]byte("x"), 256)); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit(ctx)
	if !errors.Is(err, unix.EFBIG) && !errors.Is(err, unix.ENOSPC) {
		t.Fatalf("growth: %v", err)
	}

	// Free rolls into the backed standby without replenishing it first.
	if err := db.w.free(ctx, reserved); err != nil {
		t.Fatal(err)
	}
	if st := db.cur.Load(); st.spare != 0 || st.m.table.at(reserved[0]).kind != chunkFree {
		t.Fatal("cleanup did not consume the standby and free its reservation")
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen with the same limit: replay retains acknowledged values and the
	// consumed reserve, and it cannot invent the failed transaction.
	db = openTest(t, path, Options{ReadOnly: true})
	defer func() {
		if !db.closed.Load() {
			_ = db.Close()
		}
	}()
	read, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()

	// Recovery must keep acknowledged data and omit the failed write.
	got, ok, err := read.Get(ctx, []byte("kept"))
	if err != nil || !ok || string(got) != "acknowledged" {
		t.Fatalf("acknowledged value: %q %t %v", got, ok, err)
	}
	if _, ok, err := read.Get(ctx, []byte("unacknowledged")); err != nil || ok {
		t.Fatalf("failed write: %t %v", ok, err)
	}
	if db.cur.Load().spare != 0 {
		t.Fatal("replay invented a standby")
	}

	// Restore capacity, replenish the spent standby through a normal commit,
	// then checkpoint and reopen so both superblock positions are exercised.
	read.Discard()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("S4DB_FULL_DISK_DIR") != "" {
		if err := os.Remove(filepath.Join(filepath.Dir(path), "external-fill")); err != nil {
			t.Fatal(err)
		}
	}
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &old); err != nil {
		t.Fatal(err)
	}

	// Reopen writable after restoring capacity.
	db = openTest(t, path, Options{})
	db.sched.stop()

	// A reservation made without a standby must not overlap its replenishment.
	held, err := db.w.reserve(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if held[0] == db.cur.Load().spare {
		t.Fatal("reservation overlaps the standby")
	}

	// Keep a free tail beyond the standby for the truncate rollover.
	tail, err := db.w.reserve(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.w.free(ctx, tail); err != nil {
		t.Fatal(err)
	}

	// Spend the standby on quiet marks, then fill it. The lowest free chunk is
	// now the tail being trimmed, so the truncate record must preserve its own
	// newly allocated log. Keep the lower reservation held throughout.
	for {
		st := db.cur.Load()
		mark := &record{kind: kindCommit}
		if st.spare == 0 && st.pos+uint64(recordHeader+mark.bound()+linkSize) > st.end {
			break
		}
		if err := db.w.locked(ctx, func() error { _, err := db.w.appendRecord(ctx, mark); return err }); err != nil {
			t.Fatal(err)
		}
	}

	// A release may move its own log above the target it first computed.
	if err := db.sched.releaseStep(ctx); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < fileOff(db.cur.Load().end) {
		t.Fatal("truncate removed its newly allocated log")
	}
	if err := db.w.free(ctx, held); err != nil {
		t.Fatal(err)
	}

	// Resume the real scheduler for the checkpoint stage; a stopped idle worker
	// cannot execute a flush job.
	db.sched = newScheduler(db)
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("after"), []byte("capacity restored")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if db.cur.Load().spare == 0 {
		t.Fatal("ordinary commit did not restore the standby")
	}

	// Publish the checkpoint using the active background worker.
	if err := (&flushJob{db: db}).run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the checkpoint with a fresh cache and replay state.
	db = openTest(t, path, Options{ReadOnly: true})
	read, err = db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()

	// Both acknowledged generations survive the checkpoint.
	for key, want := range map[string]string{"kept": "acknowledged", "after": "capacity restored"} {
		got, ok, err := read.Get(ctx, []byte(key))
		if err != nil || !ok || string(got) != want {
			t.Fatalf("checkpoint value %s: %q %t %v", key, got, ok, err)
		}
	}
	if db.cur.Load().spare == 0 {
		t.Fatal("checkpoint lost the restored standby")
	}
}

// fillLogDisk fills only a deliberately small private test filesystem. The
// capacity check prevents this optional test mode from filling a host filesystem.
func fillLogDisk(t *testing.T, path string) {
	// Verify the fixture's capacity before allocating any filler bytes.
	t.Helper()
	var fs unix.Statfs_t
	if err := unix.Statfs(filepath.Dir(path), &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Blocks*uint64(fs.Bsize) > 64<<20 {
		t.Fatal("full-disk fixture must be at most 64 MiB")
	}
	f, err := os.Create(filepath.Join(filepath.Dir(path), "external-fill"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Exhaust whole blocks after the large writes: ENOSPC for a 64 KiB write
	// can leave a smaller allocation available and hide an unbacked standby.
	written := 0
	for _, size := range []int{len(logZeroes), pageSize, 1} {
		for {
			_, err := f.Write(logZeroes[:size])
			if err == nil {
				err = f.Sync()
			}
			if errors.Is(err, unix.ENOSPC) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			written += size
			if written > 64<<20 {
				t.Fatal("fixture did not exhaust its capacity")
			}
		}
	}

	// Record the allocation state, including the blocks smaller writes consumed.
	if err := unix.Statfs(filepath.Dir(path), &fs); err != nil {
		t.Fatal(err)
	}
	t.Logf("full filesystem: block=%d total=%d free=%d available=%d", fs.Bsize, fs.Blocks, fs.Bfree, fs.Bavail)
	// Filesystems retain metadata or allocator reserves after even a one-byte
	// allocation returns ENOSPC. The failed write/sync defines exhaustion; the
	// reported counters are evidence rather than a portable zero-space contract.
}
