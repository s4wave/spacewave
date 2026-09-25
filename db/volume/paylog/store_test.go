//go:build !js

package paylog

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/logindex"
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

// _ checks that a Store serves a replay.
var _ workload.Target = replayTarget{}

// engine opens an index on a device.
type engine struct {
	// name names the engine.
	name string
	// open opens the index.
	open func(ctx context.Context, d device.Device) (Index, error)
}

// boltEngine is the bbolt index.
var boltEngine = engine{name: "bolt", open: OpenBolt}

// logEngine returns the log-structured index with opts.
func logEngine(opts logindex.Options) engine {
	return engine{name: "log", open: func(ctx context.Context, d device.Device) (Index, error) {
		return logindex.Open(ctx, d, opts)
	}}
}

// engines are the indexes every store test runs on.
var engines = []engine{boltEngine, logEngine(logindex.Options{})}

// openStore opens a Store on d with e.
func (e engine) openStore(ctx context.Context, d device.Device) (*Store, error) {
	index, err := e.open(ctx, d)
	if err != nil {
		return nil, err
	}
	return Open(ctx, d, index)
}

// openStore opens a Store on d with e and closes it when the test ends.
func openStore(t *testing.T, e engine, d device.Device) *Store {
	t.Helper()
	s, err := e.openStore(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestStore checks interleaved write transactions, block publication by a
// key-value commit, removes, the journal, and reopening on every engine.
func TestStore(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) { testStore(t, e) })
	}
}

// testStore runs TestStore on e.
func testStore(t *testing.T, e engine) {
	ctx := t.Context()
	d := device.NewMemory()
	s := openStore(t, e, d)

	// Two write transactions open at once both commit.
	a, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Discard()
	b, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Discard()
	if err := a.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := b.Set(ctx, []byte("b"), []byte("2")); err != nil {
		t.Fatal(err)
	}

	// A block put before a commit is published by it.
	kept, _, err := s.PutBlock(ctx, []byte("kept"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// A removed block is gone after Sync, and a duplicate put stores nothing.
	gone, _, err := s.PutBlock(ctx, []byte("gone"), &block.PutOpts{Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RmBlock(ctx, gone); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, existed, err := s.PutBlock(ctx, []byte("kept"), nil); err != nil || !existed {
		t.Fatalf("duplicate put: existed %v, err %v", existed, err)
	}

	// Journal two entries.
	edge := []block_gc.RefEdge{{Subject: "s", Object: "o"}}
	for range 2 {
		if err := s.AppendJournal(ctx, edge, nil); err != nil {
			t.Fatal(err)
		}
	}

	// Reopen and check the key-value store, the blocks, and the journal.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openStore(t, e, d)
	read, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	for key, want := range map[string]string{"a": "1", "b": "2"} {
		got, found, err := read.Get(ctx, []byte(key))
		if err != nil || !found || string(got) != want {
			t.Fatalf("get %s = %q, %v, %v", key, got, found, err)
		}
	}
	data, found, err := s.GetBlock(ctx, kept)
	if err != nil || !found || string(data) != "kept" {
		t.Fatalf("get kept block = %q, %v, %v", data, found, err)
	}
	if exists, err := s.GetBlockExists(ctx, gone); err != nil || exists {
		t.Fatalf("removed block exists %v, err %v", exists, err)
	}
	var replayed int
	err = s.ReplayJournal(ctx, func(adds, removes []block_gc.RefEdge) error {
		if len(adds) != 1 || adds[0] != edge[0] || len(removes) != 0 {
			t.Fatalf("replayed %v %v", adds, removes)
		}
		replayed++
		return nil
	})
	if err != nil || replayed != 2 {
		t.Fatalf("replayed %d entries, err %v", replayed, err)
	}
}

// TestWorkloadReplayTraces replays the captured traces named by the
// comma-separated WORKLOAD_TRACES against the store on a file device with
// every engine and logs the replay metrics. WORKLOAD_FILL_BLOCKS first fills the volume with that
// many blocks of WORKLOAD_FILL_SIZE bytes (default 1024) to measure scale.
func TestWorkloadReplayTraces(t *testing.T) {
	paths := os.Getenv("WORKLOAD_TRACES")
	if paths == "" {
		t.Skip("set WORKLOAD_TRACES to replay captured traces")
	}
	fillBlocks := envInt(t, "WORKLOAD_FILL_BLOCKS", 0)
	fillSize := envInt(t, "WORKLOAD_FILL_SIZE", 1024)
	for _, e := range engines {
		for path := range strings.SplitSeq(paths, ",") {
			t.Run(e.name+"/"+filepath.Base(path), func(t *testing.T) {
				replayTrace(t, e, path, fillBlocks, fillSize)
			})
		}
	}
}

// replayTrace replays one captured trace and logs its metrics.
func replayTrace(t *testing.T, e engine, path string, fillBlocks, fillSize int) {
	ctx := t.Context()

	// Prepare the workload and the volume it runs against.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	events, err := workload.Extract(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	records := make([]workload.Record, len(events))
	for i, ev := range events {
		records[i] = ev.Record
	}
	replay, err := workload.NewReplay(records)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := device.OpenDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	d := &countingDevice{Device: dir}
	target := replayTarget{openStore(t, e, d)}

	// Fill and seed the volume.
	fillStart := time.Now()
	if err := workload.Fill(ctx, target, fillBlocks, fillSize); err != nil {
		t.Fatal(err)
	}
	fillTime := time.Since(fillStart)
	fill := d.snapshot()
	if err := replay.Seed(ctx, target); err != nil {
		t.Fatal(err)
	}

	// Replay and measure the workload.
	seeded := d.snapshot()
	result, err := replay.Run(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	written := d.snapshot().sub(seeded)
	stored := result.BlockBytes + result.ValueBytes

	// Reopen and recover the volume.
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	openStart := time.Now()
	reopened := replayTarget{openStore(t, e, d)}
	openTime := time.Since(openStart)
	recoverStart := time.Now()
	if err := reopened.ReplayJournal(ctx); err != nil {
		t.Fatal(err)
	}
	recoverTime := time.Since(recoverStart)

	// Report.
	t.Logf("fill %d blocks of %d B in %s: %s", fillBlocks, fillSize, fillTime.Round(time.Millisecond), fill)
	t.Logf("records %d", result.Ops)
	t.Logf("wall %s, commit errors %d", result.Wall.Round(time.Microsecond), result.CommitErrors)
	for _, op := range []workload.Op{workload.OpCommit, workload.OpSync, workload.OpPutBatch, workload.OpPut, workload.OpGetBlock, workload.OpJournalAppend, workload.OpJournalReplay} {
		if n := len(result.Latency[op]); n != 0 {
			t.Logf("%-15s n %4d  p50 %9s  p99 %9s", op, n, result.Percentile(op, 0.5).Round(time.Microsecond), result.Percentile(op, 0.99).Round(time.Microsecond))
		}
	}
	t.Logf("replay wrote %s", written)
	if stored != 0 {
		t.Logf("written %d B for %d B stored (%.2fx)", written.bytes, stored, float64(written.bytes)/float64(stored))
	}
	t.Logf("open %s, recover %s", openTime.Round(time.Microsecond), recoverTime.Round(time.Microsecond))
}

// countingDevice counts the writes and flushes made through a device.
type countingDevice struct {
	device.Device

	// mtx guards stats.
	mtx sync.Mutex
	// stats holds the counts so far.
	stats deviceStats
}

// deviceStats counts device writes.
type deviceStats struct {
	// calls counts Write calls.
	calls int
	// flushes counts flushed Write calls.
	flushes int
	// bytes counts written bytes.
	bytes int64
}

// Write counts and forwards a write.
func (c *countingDevice) Write(ctx context.Context, writes []device.Write, flush bool) error {
	c.mtx.Lock()
	c.stats.calls++
	if flush {
		c.stats.flushes++
	}
	for _, w := range writes {
		c.stats.bytes += int64(len(w.Data))
	}
	c.mtx.Unlock()
	return c.Device.Write(ctx, writes, flush)
}

// snapshot returns the counts so far.
func (c *countingDevice) snapshot() deviceStats {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.stats
}

// sub returns the counts in s made after base.
func (s deviceStats) sub(base deviceStats) deviceStats {
	return deviceStats{calls: s.calls - base.calls, flushes: s.flushes - base.flushes, bytes: s.bytes - base.bytes}
}

// String formats the counts.
func (s deviceStats) String() string {
	return strconv.Itoa(s.calls) + " writes, " + strconv.Itoa(s.flushes) + " flushes, " + strconv.FormatInt(s.bytes, 10) + " B"
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
