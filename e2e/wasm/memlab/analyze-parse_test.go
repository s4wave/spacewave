//go:build !js

package memlab

import "testing"

func TestParseAnalysisResult(t *testing.T) {
	result, err := parseAnalysisResult([]byte(`{
		"snapshots": [
			{
				"label": "baseline",
				"path": "/tmp/baseline.heapsnapshot",
				"counts": {
					"clientRpc": 1,
					"channelStream": 2,
					"promise": 3,
					"generator": 4,
					"onNext": 5
				},
				"topRetained": [
					{"service": "svc.a", "method": "m1", "count": 7}
				]
			}
		],
		"deltas": {
			"clientRpc": 6,
			"channelStream": 7,
			"promise": 8,
			"generator": 9,
			"onNext": 10
		},
		"topRetained": [
			{"service": "svc.b", "method": "m2", "count": 11}
		],
		"pairDeltas": [
			{
				"service": "svc.c",
				"method": "m3",
				"baselineCount": 12,
				"finalCount": 15,
				"delta": 3
			}
		]
	}`))
	if err != nil {
		t.Fatalf("parseAnalysisResult() error = %v", err)
	}
	if len(result.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(result.Snapshots))
	}
	snapshot := result.Snapshots[0]
	if snapshot.Label != "baseline" {
		t.Fatalf("expected baseline label, got %q", snapshot.Label)
	}
	if snapshot.Path != "/tmp/baseline.heapsnapshot" {
		t.Fatalf("expected baseline path, got %q", snapshot.Path)
	}
	if snapshot.Counts.OnNext != 5 {
		t.Fatalf("expected snapshot onNext 5, got %d", snapshot.Counts.OnNext)
	}
	if len(snapshot.TopRetained) != 1 || snapshot.TopRetained[0].Count != 7 {
		t.Fatalf("expected snapshot topRetained count 7, got %+v", snapshot.TopRetained)
	}
	if result.Deltas.Promise != 8 {
		t.Fatalf("expected promise delta 8, got %d", result.Deltas.Promise)
	}
	if len(result.TopRetained) != 1 || result.TopRetained[0].Service != "svc.b" {
		t.Fatalf("expected topRetained svc.b, got %+v", result.TopRetained)
	}
	if len(result.PairDeltas) != 1 || result.PairDeltas[0].FinalCount != 15 {
		t.Fatalf("expected pair delta final count 15, got %+v", result.PairDeltas)
	}
}

func TestParseRuntimeDebugSummary(t *testing.T) {
	summary, err := parseRuntimeDebugSummary(`{
		"rpc": {
			"totalCount": 1,
			"activeCount": 2,
			"closedCount": 3,
			"activeByPair": [{"service": "svc.a", "method": "m1", "count": 4}],
			"closedByPair": [{"service": "svc.b", "method": "m2", "count": 5}]
		},
		"resource": {
			"clientTotalCount": 6,
			"clientActiveCount": 7,
			"clientDisposedCount": 8,
			"refTotalCount": 9,
			"refActiveCount": 10,
			"refReleasedCount": 11,
			"activeRefsByResourceId": [{"resourceId": 12, "count": 13}],
			"releasedRefsByResourceId": [{"resourceId": 14, "count": 15}]
		}
	}`)
	if err != nil {
		t.Fatalf("parseRuntimeDebugSummary() error = %v", err)
	}
	if summary.RPC.TotalCount != 1 || summary.RPC.ActiveCount != 2 || summary.RPC.ClosedCount != 3 {
		t.Fatalf("unexpected rpc summary: %+v", summary.RPC)
	}
	if len(summary.RPC.ActiveByPair) != 1 || summary.RPC.ActiveByPair[0].Count != 4 {
		t.Fatalf("unexpected activeByPair: %+v", summary.RPC.ActiveByPair)
	}
	if len(summary.Resource.ActiveRefsByResourceID) != 1 || summary.Resource.ActiveRefsByResourceID[0].ResourceID != 12 {
		t.Fatalf("unexpected activeRefsByResourceId: %+v", summary.Resource.ActiveRefsByResourceID)
	}
	if len(summary.Resource.ReleasedRefsByResourceID) != 1 || summary.Resource.ReleasedRefsByResourceID[0].Count != 15 {
		t.Fatalf("unexpected releasedRefsByResourceId: %+v", summary.Resource.ReleasedRefsByResourceID)
	}
}
