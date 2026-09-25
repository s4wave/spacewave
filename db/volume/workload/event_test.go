//go:build !js

package workload

import (
	"bytes"
	"context"
	"runtime/trace"
	"testing"
)

// TestExtractReadsRecordsFromExecutionTrace checks that records logged into a
// real execution trace come back in order with their fields intact, and that
// records logged outside the trace and other log categories are absent.
func TestExtractReadsRecordsFromExecutionTrace(t *testing.T) {
	ctx := context.Background()
	want := []Record{
		{Op: OpTxWrite, ID: 1},
		{Op: OpSet, ID: 1, Size: 3, Key: []byte{0x01, 'k'}},
		{Op: OpIterate, ID: 2, Parent: 1, Size: 1},
		{Op: OpGetBlock, ID: 3, Size: -1, Key: []byte{0x12, 0x20, 0xff}},
	}

	// Record the workload between trace start and stop.
	Record{Op: OpSync}.Log(ctx)
	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("start trace: %v", err)
	}
	trace.Log(ctx, "other", "sync 0 0 0 ")
	for _, rec := range want {
		rec.Log(ctx)
	}
	trace.Stop()

	// Extract and compare the records.
	events, err := Extract(&buf)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(events) != len(want) {
		t.Fatalf("extracted %d records, want %d", len(events), len(want))
	}
	for i, ev := range events {
		got, exp := ev.Record, want[i]
		if got.Op != exp.Op || got.ID != exp.ID || got.Parent != exp.Parent || got.Size != exp.Size || !bytes.Equal(got.Key, exp.Key) {
			t.Fatalf("record %d = %+v, want %+v", i, got, exp)
		}
		if i != 0 && ev.Time < events[i-1].Time {
			t.Fatalf("record %d time %d precedes record %d", i, ev.Time, i-1)
		}
	}
}
