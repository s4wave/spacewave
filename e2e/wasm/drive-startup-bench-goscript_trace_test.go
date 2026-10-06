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

	// Record a GC WAL task.
	ctx, gcTask := trace.NewTask(ctx, "hydra/block-gc/store/flush-pending/wal-append")
	trace.Logf(ctx, "hydra/block-gc/store/flush-pending/wal-append/shape", "adds=%d removes=%d", 7, 1)
	trace.Logf(ctx, "hydra/block-gc/wal/append/file", "bytes=%d files=%d", 512, 1)
	gcTask.End()

	// Record a Cayley delta task.
	ctx, graphTask := trace.NewTask(ctx, "cayley/kv/apply-deltas")
	trace.Log(ctx, "hydra/world-graph/set-quad/shape", "adds=1 duplicates=0")
	graphTask.End()

	// Record an OPFS read task.
	ctx, readTask := trace.NewTask(ctx, "hydra/opfs-engine/read")
	trace.Log(ctx, "hydra/opfs-engine/read/shape", "bytes=65536 files=1")
	readTask.End()

	// Record an OPFS publish task.
	ctx, publishTask := trace.NewTask(ctx, "hydra/opfs-engine/publish")
	trace.Log(ctx, "hydra/opfs-engine/publish/shape", "files=4 bytes=512 retired=2")
	publishTask.End()

	// Record an OPFS block-batch task.
	ctx, opfsBatchTask := trace.NewTask(ctx, "hydra/opfs-engine/block-store/put-block-batch")
	trace.Logf(ctx, "hydra/opfs-engine/block-store/put-block-batch/shape", "entries=%d bytes=%d tombstones=%d", 6, 128, 1)
	opfsBatchTask.End()

	// Stop the trace.
	trace.Stop()

	// Summarize the trace and require tasks, logs, and a shape.
	_, tasks, _, logs, _, shape := summarizeTrace(t, buf.Bytes())
	if tasks < 2 {
		t.Fatalf("tasks = %d, want at least 2", tasks)
	}
	if logs != 7 {
		t.Fatalf("logs = %d, want 7", logs)
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

	// Require the GC WAL operation fields.
	gc := findOperation(t, shape, "gc-wal")
	if gc.Count == 0 {
		t.Fatalf("gc-wal count = 0")
	}
	assertOperationField(t, gc, "wal-append.shape.adds", 7)
	assertOperationField(t, gc, "wal-append.shape.removes", 1)
	assertOperationField(t, gc, "append.file.bytes", 512)

	// Require the Cayley and OPFS read operation fields.
	cayley := findOperation(t, shape, "cayley-delta")
	if cayley.Count == 0 {
		t.Fatalf("cayley-delta count = 0")
	}
	assertOperationField(t, cayley, "set-quad.shape.adds", 1)
	read := findOperation(t, shape, "opfs-read")
	if read.Count == 0 {
		t.Fatalf("opfs-read count = 0")
	}
	assertOperationField(t, read, "shape.bytes", 65536)
	assertOperationField(t, read, "shape.files", 1)

	// Require the OPFS publish operation fields.
	publish := findOperation(t, shape, "opfs-publish")
	if publish.Count == 0 {
		t.Fatalf("opfs-publish count = 0")
	}
	assertOperationField(t, publish, "shape.files", 4)
	assertOperationField(t, publish, "shape.bytes", 512)
	assertOperationField(t, publish, "put-block-batch.shape.entries", 6)
	assertOperationField(t, publish, "put-block-batch.shape.bytes", 128)
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
