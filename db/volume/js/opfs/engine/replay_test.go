//go:build !js

package engine

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime/trace"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/volume/workload"
)

// openReplayTarget opens a fresh engine on d.
func openReplayTarget(t *testing.T, d *diskBackend) ReplayTarget {
	t.Helper()
	e, err := Open(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	s := NewBlockStore(t.Context(), e, 0)
	t.Cleanup(func() { _ = s.Close() })
	return ReplayTarget{Engine: e, BlockStore: s}
}

// traceRecords runs fn under an execution trace and returns its records.
func traceRecords(t *testing.T, fn func()) []workload.Record {
	t.Helper()
	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("start trace: %v", err)
	}
	fn()
	trace.Stop()
	events, err := workload.Extract(&buf)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	records := make([]workload.Record, len(events))
	for i, ev := range events {
		records[i] = ev.Record
	}
	return records
}

// recordOps returns the operation sequence of records.
func recordOps(records []workload.Record) []workload.Op {
	ops := make([]workload.Op, len(records))
	for i, rec := range records {
		ops[i] = rec.Op
	}
	return ops
}

// TestWorkloadReplayReproducesRecording checks that replaying a recorded
// session against a fresh engine issues the same operations with the same
// sizes, so a replay measures the workload the app produced.
func TestWorkloadReplayReproducesRecording(t *testing.T) {
	// Record the session on one engine.
	source := openReplayTarget(t, newDiskBackend(t))
	recorded := traceRecords(t, func() { runWorkloadSession(t, source.Engine, source.BlockStore) })

	// Replay it on another engine under a second trace.
	replay, err := workload.NewReplay(recorded)
	if err != nil {
		t.Fatal(err)
	}
	target := openReplayTarget(t, newDiskBackend(t))
	if err := replay.Seed(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	var result *workload.Result
	replayed := traceRecords(t, func() {
		result, err = replay.Run(t.Context(), target)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Compare the two recordings.
	if !slices.Equal(recordOps(replayed), recordOps(recorded)) {
		t.Fatalf("replayed ops = %v, want %v", recordOps(replayed), recordOps(recorded))
	}
	for i := range recorded {
		if replayed[i].Size != recorded[i].Size {
			t.Fatalf("record %d size = %d, want %d (%s)", i, replayed[i].Size, recorded[i].Size, recorded[i].Op)
		}
	}
	if result.CommitErrors != 0 || len(result.Latency[workload.OpSync]) != 1 {
		t.Fatalf("result = %+v, want no commit errors and one sync", result)
	}
}

// TestWorkloadReplayTraces replays the captured traces named by the
// comma-separated WORKLOAD_TRACES against format 3 on disk and logs the
// replay metrics. WORKLOAD_FILL_BLOCKS first fills the volume with that many
// blocks of WORKLOAD_FILL_SIZE bytes (default 1024) to measure scale.
func TestWorkloadReplayTraces(t *testing.T) {
	paths := os.Getenv("WORKLOAD_TRACES")
	if paths == "" {
		t.Skip("set WORKLOAD_TRACES to replay captured traces")
	}
	fillBlocks := envInt(t, "WORKLOAD_FILL_BLOCKS", 0)
	fillSize := envInt(t, "WORKLOAD_FILL_SIZE", 1024)
	for path := range strings.SplitSeq(paths, ",") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			replayTrace(t, path, fillBlocks, fillSize)
		})
	}
}

// replayTrace replays one captured trace and logs its metrics.
func replayTrace(t *testing.T, path string, fillBlocks, fillSize int) {
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
	d := newDiskBackend(t)
	target := openReplayTarget(t, d)

	// Fill without per-file flushes and flush the file system once, so scale
	// costs neither the fill's durability nor dirty pages inside the replay.
	fillStart := time.Now()
	d.setUnsynced(true)
	if err := workload.Fill(ctx, target, fillBlocks, fillSize); err != nil {
		t.Fatal(err)
	}
	d.setUnsynced(false)
	syscall.Sync()
	fillTime := time.Since(fillStart)
	fill := d.stats()
	if err := replay.Seed(ctx, target); err != nil {
		t.Fatal(err)
	}

	// Replay and measure the workload.
	seeded := d.stats()
	cpu := cpuTime(t)
	result, err := replay.Run(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	cpu = cpuTime(t) - cpu
	written := d.stats().sub(seeded)
	stored := result.BlockBytes + result.ValueBytes

	// Reclaim until maintenance has nothing left to do.
	before := diskUsage(t, d.root)
	reclaimStart := time.Now()
	for range 1024 {
		cleaned, err := target.CleanPack(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reclaimed, err := target.Reclaim(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !cleaned && !reclaimed {
			break
		}
	}
	reclaimTime := time.Since(reclaimStart)
	after := diskUsage(t, d.root)

	// Reopen and recover the volume.
	_ = target.BlockStore.Close()
	_ = target.Engine.Close()
	openStart := time.Now()
	reopened := openReplayTarget(t, d)
	openTime := time.Since(openStart)
	recoverStart := time.Now()
	if err := reopened.ReplayJournal(ctx, workload.DiscardJournal); err != nil {
		t.Fatal(err)
	}
	recoverTime := time.Since(recoverStart)

	// Report.
	t.Logf("fill %d blocks of %d B in %s: %s", fillBlocks, fillSize, fillTime.Round(time.Millisecond), fill)
	t.Logf("records %d", result.Ops)
	t.Logf("wall %s, cpu %s, commit errors %d", result.Wall.Round(time.Microsecond), cpu.Round(time.Microsecond), result.CommitErrors)
	for _, op := range []workload.Op{workload.OpCommit, workload.OpSync, workload.OpPutBatch, workload.OpPut, workload.OpGetBlock, workload.OpJournalAppend, workload.OpJournalReplay} {
		if n := len(result.Latency[op]); n != 0 {
			t.Logf("%-15s n %4d  p50 %9s  p99 %9s", op, n, result.Percentile(op, 0.5).Round(time.Microsecond), result.Percentile(op, 0.99).Round(time.Microsecond))
		}
	}
	t.Logf("replay wrote %s", written)
	if stored != 0 {
		t.Logf("written %d B for %d B stored (%.2fx)", written.bytes, stored, float64(written.bytes)/float64(stored))
	}
	t.Logf("disk %d B, %d B after reclaim in %s", before, after, reclaimTime.Round(time.Millisecond))
	t.Logf("open %s, recover %s", openTime.Round(time.Microsecond), recoverTime.Round(time.Microsecond))
}

// writeStats counts a disk backend's published files and bytes.
type writeStats struct {
	// files counts published files.
	files int
	// bytes counts published bytes.
	bytes int64
	// kinds counts published bytes by file kind.
	kinds map[string]int64
}

// sub returns the writes in s that happened after base.
func (s writeStats) sub(base writeStats) writeStats {
	out := writeStats{files: s.files - base.files, bytes: s.bytes - base.bytes, kinds: make(map[string]int64)}
	for kind, n := range s.kinds {
		if n -= base.kinds[kind]; n != 0 {
			out.kinds[kind] = n
		}
	}
	return out
}

// String formats the counts with the kinds in name order.
func (s writeStats) String() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(s.files) + " files, " + strconv.FormatInt(s.bytes, 10) + " B")
	for _, kind := range slices.Sorted(maps.Keys(s.kinds)) {
		b.WriteString(", " + kind + " " + strconv.FormatInt(s.kinds[kind], 10) + " B")
	}
	return b.String()
}

// stats snapshots the backend's write counters.
func (d *diskBackend) stats() writeStats {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return writeStats{files: d.files, bytes: d.written, kinds: maps.Clone(d.kindBytes)}
}

// setUnsynced switches later writes between flushed and unflushed.
func (d *diskBackend) setUnsynced(unsynced bool) {
	d.mtx.Lock()
	d.unsynced = unsynced
	d.mtx.Unlock()
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

// cpuTime returns the process's user and system CPU time.
func cpuTime(t *testing.T) time.Duration {
	t.Helper()
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// diskUsage sums the sizes of the files under root, skipping files that
// background maintenance removes during the walk.
func diskUsage(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}
