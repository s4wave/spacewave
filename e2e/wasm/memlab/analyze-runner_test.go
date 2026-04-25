//go:build !js

package memlab_test

import (
	"testing"

	"github.com/s4wave/spacewave/e2e/wasm/memlab"
)

// TestBuildSnapshotArgPreservesOrder ensures analyze.js receives snapshots
// in the same order they were captured.
func TestBuildSnapshotArgPreservesOrder(t *testing.T) {
	snapshots := &memlab.SnapshotSet{
		Labels: []string{"baseline", "action", "cleanup"},
		Snapshots: map[string]string{
			"cleanup":  "/tmp/cleanup.heapsnapshot",
			"baseline": "/tmp/baseline.heapsnapshot",
			"action":   "/tmp/action.heapsnapshot",
		},
	}

	arg, err := memlab.BuildSnapshotArg(snapshots)
	if err != nil {
		t.Fatalf("build snapshot arg: %v", err)
	}
	const expected = "baseline=/tmp/baseline.heapsnapshot,action=/tmp/action.heapsnapshot,cleanup=/tmp/cleanup.heapsnapshot"
	if arg != expected {
		t.Fatalf("expected %q, got %q", expected, arg)
	}
}

// TestSortPairDeltas sorts by descending delta, then final count, then name.
func TestSortPairDeltas(t *testing.T) {
	pairDeltas := []memlab.RetainedRpcDelta{
		{Service: "svc.b", Method: "m2", Delta: 3, FinalCount: 7},
		{Service: "svc.a", Method: "m1", Delta: 5, FinalCount: 6},
		{Service: "svc.c", Method: "m3", Delta: 5, FinalCount: 8},
		{Service: "svc.a", Method: "m0", Delta: 5, FinalCount: 8},
	}

	memlab.SortPairDeltas(pairDeltas)

	expected := []memlab.RetainedRpcDelta{
		{Service: "svc.a", Method: "m0", Delta: 5, FinalCount: 8},
		{Service: "svc.c", Method: "m3", Delta: 5, FinalCount: 8},
		{Service: "svc.a", Method: "m1", Delta: 5, FinalCount: 6},
		{Service: "svc.b", Method: "m2", Delta: 3, FinalCount: 7},
	}
	for i, expectedEntry := range expected {
		if pairDeltas[i].Service != expectedEntry.Service ||
			pairDeltas[i].Method != expectedEntry.Method ||
			pairDeltas[i].Delta != expectedEntry.Delta ||
			pairDeltas[i].FinalCount != expectedEntry.FinalCount {
			t.Fatalf("entry %d mismatch: got %+v want %+v", i, pairDeltas[i], expectedEntry)
		}
	}
}
