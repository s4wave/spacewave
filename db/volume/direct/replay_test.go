//go:build !js

package direct

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume/records"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// replayTarget drives a Store as a workload replay target.
type replayTarget struct {
	*Store
}

// ReplayJournal replays the journal into a graph that stores nothing. A
// replayed trace already carries the graph's own key-value writes.
func (t replayTarget) ReplayJournal(ctx context.Context) error {
	return t.Store.ReplayJournal(ctx, func(adds, removes []block_gc.RefEdge) error { return nil })
}

// _ checks that a Store serves a replay with ordered journal appends.
var (
	_ workload.Target         = replayTarget{}
	_ workload.OrderedJournal = replayTarget{}
)

// TestWorkloadReplayTraces replays the captured traces named by the
// comma-separated WORKLOAD_TRACES against the store on a memory record store
// and logs the record store calls each makes, once committing durably and
// once ordered. WORKLOAD_FILL_BLOCKS first fills the volume with that many
// blocks of WORKLOAD_FILL_SIZE bytes (default 1024).
func TestWorkloadReplayTraces(t *testing.T) {
	paths := os.Getenv("WORKLOAD_TRACES")
	if paths == "" {
		t.Skip("set WORKLOAD_TRACES to replay captured traces")
	}
	fillBlocks := envInt(t, "WORKLOAD_FILL_BLOCKS", 0)
	fillSize := envInt(t, "WORKLOAD_FILL_SIZE", 1024)
	for _, policy := range []string{"durable", "ordered"} {
		for path := range strings.SplitSeq(paths, ",") {
			t.Run(policy+"/"+filepath.Base(path), func(t *testing.T) {
				replayTrace(t, path, policy == "ordered", fillBlocks, fillSize)
			})
		}
	}
}

// replayTrace replays one captured trace, with ordered commits if ordered is
// set, and logs its record store calls.
func replayTrace(t *testing.T, path string, ordered bool, fillBlocks, fillSize int) {
	ctx := t.Context()

	// Prepare the workload and the volume it runs against.
	replay, err := workload.ReadTrace(path)
	if err != nil {
		t.Fatal(err)
	}
	replay.Ordered = ordered
	rs := records.NewMemory()
	s, err := Open(ctx, rs)
	if err != nil {
		t.Fatal(err)
	}
	target := replayTarget{s}

	// Fill and seed the volume.
	if err := workload.Fill(ctx, target, fillBlocks, fillSize); err != nil {
		t.Fatal(err)
	}
	if err := replay.Seed(ctx, target); err != nil {
		t.Fatal(err)
	}

	// Replay the workload.
	seeded := rs.Stats()
	result, err := replay.Run(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	stats := rs.Stats()
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and recover the volume.
	openStart := time.Now()
	reopened, err := Open(ctx, rs)
	if err != nil {
		t.Fatal(err)
	}
	openTime := time.Since(openStart)
	if err := (replayTarget{reopened}).ReplayJournal(ctx); err != nil {
		t.Fatal(err)
	}

	// Report.
	t.Logf("records %d, wall %s, ordered commits %d", result.Ops, result.Wall.Round(time.Microsecond), result.OrderedCommits)
	t.Logf(
		"replay made %d reads, %d commits (%d durable), %d B",
		stats.Reads-seeded.Reads, stats.Commits-seeded.Commits, stats.Durable-seeded.Durable, stats.Bytes-seeded.Bytes,
	)
	if stored := result.BlockBytes + result.ValueBytes; stored != 0 {
		t.Logf("written %d B for %d B stored (%.2fx)", stats.Bytes-seeded.Bytes, stored, float64(stats.Bytes-seeded.Bytes)/float64(stored))
	}
	t.Logf("open %s", openTime.Round(time.Microsecond))
}

// envInt parses an optional integer environment variable.
func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v
}
