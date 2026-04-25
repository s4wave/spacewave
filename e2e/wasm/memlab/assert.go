//go:build !js

package memlab

import (
	"testing"
)

// Thresholds defines per-type delta thresholds for leak assertions.
type Thresholds struct {
	ClientRpc     int
	ChannelStream int
	Promise       int
	Generator     int
	OnNext        int
}

// DefaultThresholds returns conservative initial thresholds.
// Calibrate after first successful run.
func DefaultThresholds() Thresholds {
	return Thresholds{
		ClientRpc:     5,
		ChannelStream: 5,
		Promise:       100,
		Generator:     50,
		OnNext:        50,
	}
}

// AssertDeltas fails the test if any delta exceeds its threshold.
func AssertDeltas(t testing.TB, result *AnalysisResult, thresholds Thresholds) {
	t.Helper()
	d := result.Deltas
	check := func(name string, delta, threshold int) {
		if delta > threshold {
			t.Errorf("LEAK: %s delta %d exceeds threshold %d", name, delta, threshold)
		} else {
			t.Logf("ok: %s delta %d (threshold %d)", name, delta, threshold)
		}
	}
	check("ClientRpc", d.ClientRpc, thresholds.ClientRpc)
	check("ChannelStream", d.ChannelStream, thresholds.ChannelStream)
	check("Promise", d.Promise, thresholds.Promise)
	check("Generator", d.Generator, thresholds.Generator)
	check("OnNext", d.OnNext, thresholds.OnNext)

	if len(result.TopRetained) > 0 {
		t.Log("top retained ClientRPC service/method pairs:")
		for _, e := range result.TopRetained {
			t.Logf("  %s/%s: %d", e.Service, e.Method, e.Count)
		}
	}
	if len(result.PairDeltas) > 0 {
		t.Log("top ClientRPC pair deltas (baseline -> final):")
		for _, e := range result.PairDeltas {
			t.Logf(
				"  %s/%s: %d -> %d (delta %+d)",
				e.Service,
				e.Method,
				e.BaselineCount,
				e.FinalCount,
				e.Delta,
			)
		}
	}
}
