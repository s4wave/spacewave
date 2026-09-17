//go:build !js && !wasip1

package world_block_engine_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
)

// Each worker is a fresh process opening the same native, synced Bolt file.
// The killed writer calls os.Exit with live engines/transactions (no cleanup),
// not a clean Close disguised as crash recovery. No storage format is replaced.
func TestWorldCommitBatchingResourceFreshProcessRecovery(t *testing.T) {
	const roleEnv = "SPACEWAVE_BATCHING_RECOVERY_ROLE"
	if role := os.Getenv(roleEnv); role != "" {
		batchingRecoveryWorker(t, role, os.Getenv("SPACEWAVE_BATCHING_RECOVERY_PATH"))
		return
	}
	for _, boundary := range []string{"prepared", "admitted", "durable"} {
		t.Run(boundary, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recovery.bolt")
			run := func(role string, wantExit int) {
				t.Helper()
				ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorldCommitBatchingResourceFreshProcessRecovery$", "-test.timeout=80s")
				cmd.Env = append(os.Environ(), roleEnv+"="+role, "SPACEWAVE_BATCHING_RECOVERY_PATH="+path)
				out, err := cmd.CombinedOutput()
				if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != wantExit || ctx.Err() != nil {
					t.Fatalf("%s exit want %d: %v\n%s", role, wantExit, err, out)
				}
			}
			run("seed", 0)
			run(boundary, 73)
			want := 1
			if boundary == "durable" {
				want = 3
			}
			run("verify-"+strconv.Itoa(want), 0)
		})
	}
}

func recoveryKeys(sample int) []string {
	keys := make([]string, 32)
	for i := range keys {
		keys[i] = fmt.Sprintf("batch/%04d/%04d", sample, i)
	}
	return keys
}

func batchingRecoveryWorker(t *testing.T, role, path string) {
	f := newBatchingFixtureAt(t, 128, path)
	ctx, cancel := context.WithTimeout(t.Context(), 70*time.Second)
	defer cancel()
	if role == "seed" {
		batchingResourceUpdate(t, f, 0, 32)
		return
	}
	if role == "verify-1" || role == "verify-3" {
		count := 1
		if role == "verify-3" {
			count = 3
		}
		for i := 0; i < count; i++ {
			got := batchingResourceReadback(t, ctx, f, recoveryKeys(i))
			want := []uint32{3919745061, 3325196325, 160724501}[i]
			if got != want {
				t.Fatalf("sample %d parity=%d want=%d", i, got, want)
			}
		}
		r, err := f.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Release()
		defer r.Discard(context.Background())
		for sample := count; sample < 3; sample++ {
			for _, key := range recoveryKeys(sample) {
				obj, found, err := r.GetObject(ctx, key)
				world.ReleaseObjectState(obj)
				if err != nil || found {
					t.Fatalf("unpublished object %q recovered: found=%v err=%v", key, found, err)
				}
			}
		}
		return
	}
	first, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	defer first.Discard(context.Background())
	initial, err := f.engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Block the first physical publication at the raw writer turn after the
	// coordinator refresh. Both admitted revisions must remain invisible.
	if role == "prepared" || role == "admitted" {
		physical, err := f.db.Begin(true)
		if err != nil {
			t.Fatal(err)
		}
		defer physical.Rollback()
	}
	before := f.db.CommitCounter()
	batchingResourcePopulate(t, ctx, f, first, 1, 32)
	if role == "prepared" {
		if n := f.db.CommitCounter() - before; n != 0 {
			t.Fatalf("prepared state wrote %d commits", n)
		}
		os.Exit(73)
	}
	done1 := make(chan error, 1)
	go func() { done1 <- first.Commit(ctx) }()
	second, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	defer second.Discard(context.Background())
	batchingResourcePopulate(t, ctx, f, second, 2, 32)
	done2 := make(chan error, 1)
	go func() { done2 <- second.Commit(ctx) }()
	probe, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Discard(ctx); err != nil {
		t.Fatal(err)
	}
	probe.Release()
	if role == "admitted" {
		seq, err := f.engine.GetSeqno(ctx)
		if err != nil || seq != initial {
			t.Fatalf("canonical head changed: %d != %d: %v", seq, initial, err)
		}
		for _, ch := range []<-chan error{done1, done2} {
			select {
			case err := <-ch:
				t.Fatalf("early durability: %v", err)
			default:
			}
		}
		if n := f.db.CommitCounter() - before; n != 0 {
			t.Fatalf("admission wrote %d commits", n)
		}
		os.Exit(73)
	}
	if role != "durable" {
		t.Fatal("unknown recovery role", role)
	}
	for _, ch := range []<-chan error{done1, done2} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	// The acknowledgement itself, not process/engine shutdown, is the fence.
	os.Exit(73)
}
