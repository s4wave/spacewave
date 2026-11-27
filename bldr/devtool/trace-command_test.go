//go:build !js

package devtool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	stdtrace "runtime/trace"
	"strings"
	"testing"

	bldr_trace "github.com/s4wave/spacewave/bldr/devtool/trace"
)

func TestTraceMinDurFilterPreservesLogs(t *testing.T) {
	logs := []*bldr_trace.LogStats{{Category: "startup", Message: "skipped", Count: 2}}
	result := &bldr_trace.TraceResult{
		TypeStats: []*bldr_trace.TaskTypeStats{{Name: "keep"}, {Name: "drop"}},
		Logs:      logs,
	}

	filtered := filterTraceByMinDur(result, 1)
	if len(filtered.Logs) != 1 || filtered.Logs[0] != logs[0] {
		t.Fatalf("filterTraceByMinDur logs = %#v", filtered.Logs)
	}
}

func TestTraceCommandWritesTasks(t *testing.T) {
	// Record a trace containing one task with a child.
	var data bytes.Buffer
	if err := stdtrace.Start(&data); err != nil {
		t.Fatal(err)
	}
	ctx, outer := stdtrace.NewTask(context.Background(), "app/request")
	_, inner := stdtrace.NewTask(ctx, "db/read")
	inner.End()
	outer.End()
	stdtrace.Stop()

	// Write the trace to a file.
	path := filepath.Join(t.TempDir(), "trace.out")
	if err := os.WriteFile(path, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// The table and the tree both list the tasks.
	var out bytes.Buffer
	args := &traceArgs{table: true, tree: true, top: 30}
	if err := args.run(&out, path); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "2 tasks") || strings.Count(got, "app/request") < 2 || strings.Count(got, "db/read") < 2 {
		t.Fatalf("trace output:\n%s", got)
	}
}
