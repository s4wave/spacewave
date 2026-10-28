//go:build !js

package sobject_world_engine_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// TestWorldCommitLogsWholeWorldReads checks that a slow write that read the
// root World state reports a whole-World read set.
func TestWorldCommitLogsWholeWorldReads(t *testing.T) {
	// Capture the warnings, then read the root World state in a write.
	w := startLocalWorld(t, t.TempDir(), nil)
	hook := logtest.NewLocal(w.le.Logger)
	defer sobject_world_engine.SetSlowCommitThreshold(0)()
	tx := w.newWrite(t, "reads/0")
	if err := tx.AccessWorldState(t.Context(), nil, func(*bucket_lookup.Cursor) error { return nil }); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	// The warning names the whole World as read.
	entry := hook.LastEntry()
	if entry == nil || entry.Message != "slow World commit" {
		t.Fatalf("last entry is %v, want the slow World commit warning", entry)
	}
	if reads := entry.Data["reads"]; reads != "world" {
		t.Fatalf("reads field is %v, want world", reads)
	}
}

// TestWorldCommitLogsStageTimes lowers the slow threshold to zero and checks
// that a World write logs one warning that names every stage with its
// duration, and the writer lock hold, which excludes the receipt wait.
func TestWorldCommitLogsStageTimes(t *testing.T) {
	// Capture the warnings the engine logs, then commit one write.
	w := startLocalWorld(t, t.TempDir(), nil)
	hook := logtest.NewLocal(w.le.Logger)
	defer sobject_world_engine.SetSlowCommitThreshold(0)()
	w.create(t, "stages/0")

	// One warning is logged.
	var slow []*logrus.Entry
	for _, entry := range hook.AllEntries() {
		if entry.Message == "slow World commit" {
			slow = append(slow, entry)
		}
	}
	if len(slow) != 1 {
		t.Fatalf("got %d slow World commit warnings, want 1", len(slow))
	}

	// It carries the elapsed time, the hold and every stage with its duration,
	// in stage order.
	elapsed, ok := slow[0].Data["elapsed"].(time.Duration)
	if !ok {
		t.Fatalf("warning lacks the elapsed duration: %v", slow[0].Data)
	}
	names := []string{
		"lock-wait", "fork-replay", "callback", "world-state-commit",
		"block-commit", "publication-admission", "advance", "receipt-wait",
	}
	hold, ok := slow[0].Data["hold"].(time.Duration)
	if !ok {
		t.Fatalf("warning lacks the hold duration: %v", slow[0].Data)
	}
	fields := strings.Fields(fmt.Sprint(slow[0].Data["stages"]))
	if len(fields) != len(names) {
		t.Fatalf("stages field has %d entries, want %d: %v", len(fields), len(names), fields)
	}

	// A write that named its object read only keys.
	if reads := slow[0].Data["reads"]; reads != "keys" {
		t.Fatalf("reads field is %v, want keys for a write that named its object", reads)
	}

	// The stage durations add up to no more than the elapsed time of the
	// write, and the hold is the stages between the lock wait and the receipt
	// wait.
	var sum, held time.Duration
	for i, field := range fields {
		name, value, _ := strings.Cut(field, "=")
		if name != names[i] {
			t.Fatalf("stage %d is %s, want %s", i, name, names[i])
		}
		d, err := time.ParseDuration(value)
		if err != nil {
			t.Fatalf("stage %s has duration %q: %v", name, value, err)
		}
		sum += d
		if i != 0 && i != len(fields)-1 {
			held += d
		}
	}
	if sum > elapsed {
		t.Fatalf("stages sum to %v, more than the elapsed %v", sum, elapsed)
	}
	if held != hold {
		t.Fatalf("hold is %v, want the %v the held stages sum to", hold, held)
	}
}

// TestWorldCommitReportsObserver checks that a write opened with a commit
// observer reports its stage times once, every stage by name, with the hold
// summed over the held stages, while a write without one reports nothing.
func TestWorldCommitReportsObserver(t *testing.T) {
	// Open a write whose context holds an observer.
	w := startLocalWorld(t, t.TempDir(), nil)
	var got []sobject_world_engine.CommitTimes
	ctx := sobject_world_engine.WithCommitObserver(t.Context(), func(times sobject_world_engine.CommitTimes) {
		got = append(got, times)
	})

	// Commit it, then commit one write without an observer.
	tx, err := w.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := tx.CreateObject(ctx, "observed/0", &bucket.ObjectRef{BucketId: "local-world-bucket"})
	world.ReleaseObjectState(obj)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	w.create(t, "observed/1")

	// The observed write reports every stage, and its hold excludes the lock
	// and receipt waits.
	if len(got) != 1 {
		t.Fatalf("observer ran %d times, want 1", len(got))
	}
	times := got[0]
	names := []string{
		"lock-wait", "fork-replay", "callback", "world-state-commit",
		"block-commit", "publication-admission", "advance", "receipt-wait",
	}
	if len(times.Stages) != len(names) {
		t.Fatalf("got %d stages, want %d", len(times.Stages), len(names))
	}
	var sum, held time.Duration
	for i, st := range times.Stages {
		if st.Name != names[i] {
			t.Fatalf("stage %d is %s, want %s", i, st.Name, names[i])
		}
		sum += st.Duration
		if i != 0 && i != len(names)-1 {
			held += st.Duration
		}
	}
	if sum > times.Elapsed {
		t.Fatalf("stages sum to %v, more than the elapsed %v", sum, times.Elapsed)
	}
	if held != times.Hold {
		t.Fatalf("hold is %v, want the %v the held stages sum to", times.Hold, held)
	}
}
