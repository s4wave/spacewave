package s4db

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// releaseEnv selects the real producer process that leaves a large reservation.
const releaseEnv = "S4DB_TEST_RELEASE_BACKLOG"

// TestReleaseBacklog drains several bounded batches through the real scheduler,
// without charging one quiet timer per batch or growing a sparse reservation.
func TestReleaseBacklog(t *testing.T) {
	// The producer exits with durable unused reservations under its own identity.
	if path := os.Getenv(releaseEnv); path != "" {
		db := openTest(t, path, Options{})
		db.sched.stop()
		if _, err := db.w.reserve(t.Context(), 8*freeBatch); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Run a real second owner; its exit makes the reservation reclaimable.
	path := filepath.Join(t.TempDir(), "backlog.s4wave")
	db := openTest(t, path, Options{})
	defer db.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*quietMark)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReleaseBacklog$", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), releaseEnv+"="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("producer: %v\n%s", err, out)
	}

	// Start the first bounded batch. The scheduler must retain the continuation
	// instead of making the remaining seven batches wait for quiet timers.
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.sched.releaseStep(ctx); err != nil {
		t.Fatal(err)
	}
	err = db.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		for _, e := range db.cur.Load().m.table.entries {
			if e.kind == chunkReserved {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("release continuation stalled: %v", err)
	}

	// Logical reservations never justified extending the physical file.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() > before.Size() {
		t.Fatal("release grew the sparse reservation")
	}
}
