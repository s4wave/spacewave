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

func TestSummarizeTraceBuildsOperationShapeFromTasksAndLogs(t *testing.T) {
	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("start trace: %v", err)
	}

	ctx := context.Background()
	ctx, blockTask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root")
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/write-shape", "encoded_blocks=%d put_blocks=%d", 3, 2)
	blockTask.End()

	ctx, gcTask := trace.NewTask(ctx, "hydra/block-gc/store/flush-pending/wal-append")
	trace.Logf(ctx, "hydra/block-gc/store/flush-pending/wal-append/shape", "adds=%d removes=%d", 7, 1)
	trace.Logf(ctx, "hydra/block-gc/wal/append/file", "bytes=%d files=%d", 512, 1)
	gcTask.End()

	ctx, graphTask := trace.NewTask(ctx, "cayley/kv/apply-deltas")
	trace.Log(ctx, "hydra/world-graph/set-quad/shape", "adds=1 duplicates=0")
	graphTask.End()

	ctx, publishTask := trace.NewTask(ctx, "hydra/opfs-blockshard/run-actor/publish")
	trace.Log(ctx, "coalesce", "reqs=4 entries=5")
	publishTask.End()

	ctx, opfsBatchTask := trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/put-block-batch")
	trace.Logf(ctx, "hydra/opfs-blockshard/block-store/put-block-batch/shape", "entries=%d bytes=%d tombstones=%d", 6, 128, 1)
	opfsBatchTask.End()

	trace.Stop()

	_, tasks, _, logs, _, shape := summarizeTrace(t, buf.Bytes())
	if tasks < 2 {
		t.Fatalf("tasks = %d, want at least 2", tasks)
	}
	if logs != 6 {
		t.Fatalf("logs = %d, want 6", logs)
	}
	if shape == nil {
		t.Fatal("operation shape is nil")
	}

	block := findOperation(t, shape, "block-write")
	if block.Count == 0 {
		t.Fatalf("block-write count = 0")
	}
	assertOperationField(t, block, "write-shape.encoded_blocks", 3)
	assertOperationField(t, block, "write-shape.put_blocks", 2)

	gc := findOperation(t, shape, "gc-wal")
	if gc.Count == 0 {
		t.Fatalf("gc-wal count = 0")
	}
	assertOperationField(t, gc, "wal-append.shape.adds", 7)
	assertOperationField(t, gc, "wal-append.shape.removes", 1)
	assertOperationField(t, gc, "append.file.bytes", 512)

	cayley := findOperation(t, shape, "cayley-delta")
	if cayley.Count == 0 {
		t.Fatalf("cayley-delta count = 0")
	}
	assertOperationField(t, cayley, "set-quad.shape.adds", 1)
	publish := findOperation(t, shape, "opfs-publish")
	if publish.Count == 0 {
		t.Fatalf("opfs-publish count = 0")
	}
	assertOperationField(t, publish, "coalesce.reqs", 4)
	assertOperationField(t, publish, "coalesce.entries", 5)
	assertOperationField(t, publish, "put-block-batch.shape.entries", 6)
	assertOperationField(t, publish, "put-block-batch.shape.bytes", 128)
}

func TestSummarizeBrowserCPUProfileBucketsSamples(t *testing.T) {
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
					"functionName": "Publish",
					"url":          "https://example.invalid/db/volume/js/opfs/blockshard/engine.gs.js",
				},
			},
		},
		"samples":    []any{2, 3},
		"timeDeltas": []any{100, 250},
	}

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

	data := marshalBrowserProfileJSON(profile)
	if err := fastjson.ValidateBytes(data); err != nil {
		t.Fatalf("profile JSON invalid: %v", err)
	}
}

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
