package world_block_test

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

func TestWorldCommitGCFlushCostByStoreSize(t *testing.T) {
	if os.Getenv("SPACEWAVE_GC_FLUSH_COST") == "" {
		t.Skip("set SPACEWAVE_GC_FLUSH_COST=1 to run the timed harness")
	}

	trialValues := make(map[int][]time.Duration, len(worldCommitCostCheckpoints))
	for trial := 1; trial <= worldCommitCostTrialCount; trial++ {
		values := runWorldCommitCostTrial(t)
		for _, checkpoint := range worldCommitCostCheckpoints {
			value := values[checkpoint]
			trialValues[checkpoint] = append(trialValues[checkpoint], value)
			t.Logf(
				"trial=%d commits=%d trailing_%d_avg=%s",
				trial,
				checkpoint,
				worldCommitCostWindow,
				value,
			)
		}
	}

	var minMedian, maxMedian time.Duration
	for _, checkpoint := range worldCommitCostCheckpoints {
		samples := slices.Clone(trialValues[checkpoint])
		slices.Sort(samples)
		median := samples[len(samples)/2]
		if minMedian == 0 || median < minMedian {
			minMedian = median
		}
		if median > maxMedian {
			maxMedian = median
		}
		t.Logf(
			"median commits=%d trials=%d trailing_%d_avg=%s",
			checkpoint,
			len(samples),
			worldCommitCostWindow,
			median,
		)
	}
	ratio := float64(maxMedian) / float64(minMedian)
	t.Logf(
		"cost_gate max_median=%s min_median=%s ratio=%.3f limit=3.000",
		maxMedian,
		minMedian,
		ratio,
	)
	if os.Getenv("SPACEWAVE_GC_FLUSH_GATE") != "" && ratio > 3 {
		t.Fatalf(
			"GC flush cost gate failed: max median %s / min median %s = %.3f > 3.000",
			maxMedian,
			minMedian,
			ratio,
		)
	}
}

const (
	worldCommitCostTrialCount = 7
	worldCommitCostWindow     = 8
)

var worldCommitCostCheckpoints = []int{1, 8, 16, 32, 64, 96, 128}

// runWorldCommitCostTrial commits 128 times and returns the trailing average
// commit time at each checkpoint.
func runWorldCommitCostTrial(t *testing.T) map[int]time.Duration {
	// Build a World that tracks its GC graph.
	t.Helper()
	ctx := context.Background()
	ws, cleanup := setupWorldState(ctx, t)
	defer cleanup()

	// Index the checkpoints and prepare the trailing window.
	checkpoints := make(map[int]struct{}, len(worldCommitCostCheckpoints))
	for _, checkpoint := range worldCommitCostCheckpoints {
		checkpoints[checkpoint] = struct{}{}
	}
	windowDurations := make([]time.Duration, 0, worldCommitCostWindow)
	values := make(map[int]time.Duration, len(checkpoints))

	// Commit one new object at a time and sample the commit cost.
	for i := 1; i <= 128; i++ {
		// Create one object and time its commit.
		createdObject, err := world_block.BuildMockObject(ctx, ws, "gc-flush-cost/"+strconv.Itoa(i))
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
		start := time.Now()
		if err := ws.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}

		// Keep the last window of commit times.
		windowDurations = append(windowDurations, time.Since(start))
		if len(windowDurations) > worldCommitCostWindow {
			windowDurations = windowDurations[1:]
		}

		// Record the window average at each checkpoint.
		if _, ok := checkpoints[i]; ok {
			var total time.Duration
			for _, sample := range windowDurations {
				total += sample
			}
			values[i] = total / time.Duration(len(windowDurations))
		}
	}
	return values
}
