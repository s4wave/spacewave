//go:build !goscript

package alarm_volume

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	alarm_watch "github.com/s4wave/spacewave/sdk/alarm/watch"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	function_run "github.com/s4wave/spacewave/sdk/function/run"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// TestDefinitions checks that each definition's queries parse, that its node
// parameters compile to a valid watch config, and that its function clears
// with no series and fires naming each volume with one.
func TestDefinitions(t *testing.T) {
	// Run each function with one runner.
	ctx := t.Context()
	runner := function_run.NewRunner()
	firing := map[string]struct {
		series  *s4wave_prom.Series
		summary string
	}{
		"volume-gc-stuck":     {series("vol/a", "mark", 2000), "vol/a stuck in mark"},
		"volume-reserve-soon": {series("vol/a", "", 5.2), "vol/a reaches its reserve in 6 h"},
		"volume-reserve":      {series("vol/a", "", 1), "vol/a refuses writes below its reserve"},
	}
	defs := Definitions(Options{})
	if len(defs) != len(firing) {
		t.Fatalf("%d definitions", len(defs))
	}
	for _, def := range defs {
		// Parse the queries.
		want, ok := firing[def.Name]
		if !ok {
			t.Fatalf("unexpected definition %q", def.Name)
		}
		for _, q := range def.Queries {
			if _, err := parser.NewParser(parser.Options{}).ParseExpr(q.GetExpr()); err != nil {
				t.Fatalf("%s query %s: %v", def.Name, q.GetName(), err)
			}
		}

		// The node parameters compile to the definition's watch.
		conf, err := alarm_watch.NewConfigFromNode(&s4wave_flowgraph.PlacedFlowgraphNode{
			EngineID: "engine",
			Node:     &s4wave_flowgraph.FlowgraphNode{ObjectKey: "alarms/" + def.Name, Parameters: def.NodeParameters("functions/"+def.Name, "prom/target/device")},
		})
		if err != nil {
			t.Fatalf("%s node: %v", def.Name, err)
		}
		if conf.GetTitle() != def.Title || conf.GetHoldMs() != def.Hold.Milliseconds() || len(conf.GetQueries()) != len(def.Queries) {
			t.Fatalf("%s config %v", def.Name, conf)
		}

		// The function clears with no series and fires with one.
		fn, err := def.Function()
		if err != nil {
			t.Fatalf("%s compile: %v", def.Name, err)
		}
		name := def.Queries[0].GetName()
		for _, in := range []*s4wave_prom.QueryResponse{{}, {Series: []*s4wave_prom.Series{want.series}}} {
			var out alarm_watch.WatchResult
			input := &alarm_watch.WatchInput{TimeMs: time.Now().UnixMilli(), Queries: map[string]*s4wave_prom.QueryResponse{name: in}}
			if err := runner.Run(ctx, fn, input, &out); err != nil {
				t.Fatalf("%s run: %v", def.Name, err)
			}
			if fire := len(in.GetSeries()) != 0; out.GetFiring() != fire || (fire && out.GetSummary() != want.summary) {
				t.Fatalf("%s with %d series: %v", def.Name, len(in.GetSeries()), out.String())
			}
		}
	}
}

// TestOptions checks that the thresholds reach the queries and the hold.
func TestOptions(t *testing.T) {
	// Set every threshold, then read each back.
	defs := Definitions(Options{GCStuckAfter: 1500 * time.Millisecond, ReserveWithin: 90 * time.Minute, ReserveSoonHold: time.Minute})
	if got := defs[0].Queries[0].GetExpr(); !strings.HasSuffix(got, "> 1.5") {
		t.Fatalf("stuck query %q", got)
	}
	if got := defs[1].Queries[0].GetExpr(); !strings.HasSuffix(got, "< 1.5") || defs[1].Hold != time.Minute {
		t.Fatalf("reserve soon %q hold %v", got, defs[1].Hold)
	}
}

// series returns a query series for volume, with phase when set, whose last
// sample is value.
func series(volume, phase string, value float64) *s4wave_prom.Series {
	labels := []*s4wave_prom.Label{{Name: "volume", Value: volume}}
	if phase != "" {
		labels = append(labels, &s4wave_prom.Label{Name: "phase", Value: phase})
	}
	return &s4wave_prom.Series{
		Labels:  &s4wave_prom.LabelSet{Labels: labels},
		Samples: []*s4wave_prom.Sample{{Value: value}},
	}
}
