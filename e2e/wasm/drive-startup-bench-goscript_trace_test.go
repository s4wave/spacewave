//go:build !skip_e2e && !js

package wasm

import (
	"bytes"
	"context"
	"runtime/trace"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/e2e/drivebench"
)

// TestSummarizeTraceBuildsOperationShapeFromTasksAndLogs checks operation counts and numeric fields from a real Go trace.
func TestSummarizeTraceBuildsOperationShapeFromTasksAndLogs(t *testing.T) {
	// Start a trace buffer.
	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("start trace: %v", err)
	}

	// Record a block-write task.
	ctx := context.Background()
	ctx, blockTask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root")
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/write-shape", "encoded_blocks=%d put_blocks=%d", 3, 2)
	blockTask.End()

	// Record a World commit task.
	ctx, commitTask := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction")
	trace.Logf(ctx, "hydra/world-block/engine-tx/commit-block-transaction/shape", "blocks=%d roots=%d", 7, 1)
	commitTask.End()

	// Record a Cayley delta task.
	ctx, graphTask := trace.NewTask(ctx, "cayley/kv/apply-deltas")
	trace.Log(ctx, "cayley/kv/apply-deltas/shape", "adds=1 duplicates=0")
	graphTask.End()

	// Record a Space replay task.
	ctx, replayTask := trace.NewTask(ctx, "space/world/replay/apply")
	trace.Log(ctx, "space/world/replay/apply/shape", "ops=4 refused=1")
	replayTask.End()

	// Record an OPFS write task.
	ctx, writeTask := trace.NewTask(ctx, "hydra/opfs/async-file/write-at")
	trace.Logf(ctx, "hydra/opfs/async-file/write-at/shape", "bytes=%d files=%d", 65536, 1)
	writeTask.End()

	// Stop the trace.
	trace.Stop()

	// Summarize the trace and require tasks, logs, and a shape.
	_, tasks, _, logs, _, shape := summarizeTrace(t, buf.Bytes())
	if tasks < 2 {
		t.Fatalf("tasks = %d, want at least 2", tasks)
	}
	if logs != 5 {
		t.Fatalf("logs = %d, want 5", logs)
	}
	if shape == nil {
		t.Fatal("operation shape is nil")
	}

	// Require the block-write operation fields.
	block := findOperation(t, shape, "block-write")
	if block.Count == 0 {
		t.Fatalf("block-write count = 0")
	}
	assertOperationField(t, block, "write-shape.encoded_blocks", 3)
	assertOperationField(t, block, "write-shape.put_blocks", 2)

	// Require the World commit operation fields.
	commit := findOperation(t, shape, "write-transaction")
	if commit.Count == 0 {
		t.Fatalf("write-transaction count = 0")
	}
	assertOperationField(t, commit, "commit-block-transaction.shape.blocks", 7)
	assertOperationField(t, commit, "commit-block-transaction.shape.roots", 1)

	// Require the Cayley, Space replay and OPFS operation fields.
	cayley := findOperation(t, shape, "cayley-delta")
	if cayley.Count == 0 {
		t.Fatalf("cayley-delta count = 0")
	}
	assertOperationField(t, cayley, "shape.adds", 1)
	replay := findOperation(t, shape, "space-replay")
	if replay.Count == 0 {
		t.Fatalf("space-replay count = 0")
	}
	assertOperationField(t, replay, "apply.shape.ops", 4)
	assertOperationField(t, replay, "apply.shape.refused", 1)
	opfs := findOperation(t, shape, "opfs-file")
	if opfs.Count == 0 {
		t.Fatalf("opfs-file count = 0")
	}
	assertOperationField(t, opfs, "async-file.write-at.shape.bytes", 65536)
	assertOperationField(t, opfs, "async-file.write-at.shape.files", 1)
}

// TestSummarizeBrowserCPUProfileBucketsSamples checks self time, inclusive time, and valid profile serialization.
func TestSummarizeBrowserCPUProfileBucketsSamples(t *testing.T) {
	// Build a CPU profile with GoScript, OPFS, and browser frames.
	profile := map[string]any{
		"nodes": []any{
			map[string]any{
				"id": 1,
				"callFrame": map[string]any{
					"functionName": "(root)",
					"url":          "",
				},
				"children": []any{2, 3},
			},
			map[string]any{
				"id": 2,
				"callFrame": map[string]any{
					"functionName": "$.chanSend",
					"url":          "https://example.invalid/gs/builtin/channel.js",
				},
			},
			map[string]any{
				"id": 3,
				"callFrame": map[string]any{
					"functionName": "Write",
					"url":          "https://example.invalid/db/volume/device/opfs/opfs.gs.js",
				},
			},
		},
		"samples":    []any{2, 3},
		"timeDeltas": []any{100, 250},
	}

	// Bucket the samples and require their times.
	buckets := summarizeBrowserCPUProfile(profile)
	goscript := findProfileBucket(t, buckets, "goscript-runtime")
	if goscript.Count != 1 || goscript.SelfUs != 100 || goscript.TotalUs != 100 {
		t.Fatalf("goscript bucket = %+v", goscript)
	}
	opfs := findProfileBucket(t, buckets, "storage-opfs")
	if opfs.Count != 1 || opfs.SelfUs != 250 || opfs.TotalUs != 250 {
		t.Fatalf("opfs bucket = %+v", opfs)
	}
	browser := findProfileBucket(t, buckets, "browser-runtime")
	if browser.SelfUs != 0 || browser.TotalUs != 350 {
		t.Fatalf("browser bucket = %+v", browser)
	}

	// Require the profile JSON to parse.
	data := marshalBrowserProfileJSON(profile)
	if err := fastjson.ValidateBytes(data); err != nil {
		t.Fatalf("profile JSON invalid: %v", err)
	}
}

// findProfileBucket requires a named CPU-profile bucket.
func findProfileBucket(t testing.TB, buckets []drivebench.ProfileBucket, name string) drivebench.ProfileBucket {
	t.Helper()
	for _, bucket := range buckets {
		if bucket.Name == name {
			return bucket
		}
	}
	t.Fatalf("profile bucket %q not found in %#v", name, buckets)
	return drivebench.ProfileBucket{}
}

// findOperation requires a named operation summary.
func findOperation(t testing.TB, shape *drivebench.OperationShape, name string) drivebench.OperationSummary {
	t.Helper()
	for _, op := range shape.Operations {
		if op.Name == name {
			return op
		}
	}
	t.Fatalf("operation %q not found in %#v", name, shape.Operations)
	return drivebench.OperationSummary{}
}

// assertOperationField requires one exact numeric trace-field observation.
func assertOperationField(t testing.TB, op drivebench.OperationSummary, name string, want int64) {
	t.Helper()
	for _, field := range op.Fields {
		if field.Name != name {
			continue
		}
		if field.Samples != 1 || field.Sum != want || field.Max != want || field.Last != want {
			t.Fatalf("field %s = %+v, want one sample %d", name, field, want)
		}
		return
	}
	t.Fatalf("field %q not found in %+v", name, op.Fields)
}
