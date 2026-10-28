//go:build !js

package sobject_world_engine_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/metrics"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// TestCommitFlowsScrape checks that a published World commit reports its
// World pages, payload, state and replay save in the metrics registry, that a
// stable point not checkpointed counts its reason, and that a checkpoint
// written counts.
func TestCommitFlowsScrape(t *testing.T) {
	// Commit an object with a root block to a new Space.
	ctx := t.Context()
	w := newSpaceWorld(ctx, t)
	before := scrapeWorld(t)
	write := func(ctx context.Context, ws world.WorldState) error {
		return writeExample(ctx, ws, "record", "created")
	}
	if err := world.ExecTransaction(ctx, w.eng, true, write); err != nil {
		t.Fatal(err)
	}
	after := scrapeWorld(t)

	// The publication carried each kind of record.
	grew := func(key string) bool { return after[key] > before[key] }
	for _, key := range []string{
		"spacewave_world_commit_publications_total",
		"spacewave_world_commit_blocks_total{kind=world}",
		"spacewave_world_commit_bytes_total{kind=world}",
		"spacewave_world_commit_blocks_total{kind=payload}",
		"spacewave_world_commit_bytes_total{kind=payload}",
		"spacewave_world_commit_bytes_total{kind=state}",
		"spacewave_world_commit_bytes_total{kind=replay}",
	} {
		if !grew(key) {
			t.Fatalf("%s did not grow: %v before, %v after", key, before[key], after[key])
		}
	}

	// One operation is fewer than a checkpoint needs, so its stable point is
	// skipped for that reason.
	if key := "spacewave_world_checkpoint_skips_total{reason=few-stable}"; !grew(key) {
		t.Fatalf("%s did not grow: %v before, %v after", key, before[key], after[key])
	}

	// Once enough operations are stable, the local checkpointer writes one.
	for i := range sobject.MinCheckpointOperations {
		write := func(ctx context.Context, ws world.WorldState) error {
			return writeExample(ctx, ws, "record", "edit "+strconv.Itoa(i))
		}
		if err := world.ExecTransaction(ctx, w.eng, true, write); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for !grew("spacewave_world_checkpoints_total{outcome=written}") {
		if time.Now().After(deadline) {
			t.Fatal("no checkpoint written")
		}
		time.Sleep(10 * time.Millisecond)
		after = scrapeWorld(t)
	}
}

// scrapeWorld gathers the process metrics and returns the World commit
// counters, keyed by name and any kind, outcome or reason label.
func scrapeWorld(t *testing.T) map[string]float64 {
	// Gather the families.
	t.Helper()
	families, err := metrics.Gather()
	if err != nil {
		t.Fatal(err)
	}

	// Keep the World series.
	values := make(map[string]float64)
	for _, f := range families {
		for _, m := range f.GetMetric() {
			key := f.GetName()
			for _, l := range m.GetLabel() {
				key += "{" + l.GetName() + "=" + l.GetValue() + "}"
			}
			values[key] = m.GetCounter().GetValue()
		}
	}
	return values
}
