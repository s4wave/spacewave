package world_block_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	world_block "github.com/s4wave/spacewave/db/world/block"
)

func TestWorldCommitCostByStoreSize(t *testing.T) {
	if os.Getenv("SPACEWAVE_GC_FLUSH_COST") == "" {
		t.Skip("set SPACEWAVE_GC_FLUSH_COST=1 to run the timed harness")
	}

	ctx := context.Background()
	ws, cleanup := setupWorldWriteBench(ctx, t)
	defer cleanup()

	checkpoints := map[int]struct{}{
		1: {}, 8: {}, 16: {}, 32: {}, 64: {}, 96: {}, 128: {},
	}
	const window = 8
	windowDurations := make([]time.Duration, 0, window)
	for i := 1; i <= 128; i++ {
		if _, err := world_block.BuildMockObject(ctx, ws, "gc-flush-cost/"+strconv.Itoa(i)); err != nil {
			t.Fatal(err.Error())
		}
		start := time.Now()
		if err := ws.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}
		duration := time.Since(start)
		windowDurations = append(windowDurations, duration)
		if len(windowDurations) > window {
			windowDurations = windowDurations[1:]
		}
		if _, ok := checkpoints[i]; !ok {
			continue
		}
		var windowTotal time.Duration
		for _, sample := range windowDurations {
			windowTotal += sample
		}
		t.Logf("commits=%d commit=%s trailing_%d_avg=%s", i, duration, len(windowDurations), windowTotal/time.Duration(len(windowDurations)))
	}
}
