package alarm_watch

import (
	"context"
	"testing"

	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// TestCompile compiles a watch node into its controller config.
func TestCompile(t *testing.T) {
	// Place a node bound to its Alarm, with two queries and two objects.
	node := &s4wave_flowgraph.PlacedFlowgraphNode{
		FlowgraphKey: "flowgraphs/forge",
		NodeID:       "watch",
		EngineID:     "engine",
		Node: &s4wave_flowgraph.FlowgraphNode{
			TypeId:    NodeTypeID,
			ObjectKey: "alarms/forge-down",
			Parameters: map[string]string{
				ParamFunction:             "functions/forge-down",
				ParamHold:                 "2m",
				ParamTitle:                "Forge down",
				ParamSource:               "prom/forge",
				ParamQueryPrefix + "up":   "up",
				ParamQueryPrefix + "disk": "node_filesystem_avail_bytes",
				ParamObjects:              "repos/a, repos/b,",
			},
		},
	}

	// The config names every input, with the queries in name order.
	entries, err := NodeType{}.Compile(context.Background(), node)
	if err != nil {
		t.Fatal(err)
	}
	want := &Config{
		EngineId:    "engine",
		AlarmKey:    "alarms/forge-down",
		FunctionKey: "functions/forge-down",
		HoldMs:      120_000,
		Title:       "Forge down",
		SourceKey:   "prom/forge",
		Queries:     []*Query{{Name: "disk", Expr: "node_filesystem_avail_bytes"}, {Name: "up", Expr: "up"}},
		ObjectKeys:  []string{"repos/a", "repos/b"},
	}
	if got, ok := entries["watch"].(*Config); !ok || len(entries) != 1 || !got.EqualVT(want) {
		t.Fatalf("compiled %v, want %v", entries, want)
	}

	// A node with no Alarm, a bad hold or queries with no source is rejected.
	for name, change := range map[string]func(n *s4wave_flowgraph.FlowgraphNode){
		"no alarm":  func(n *s4wave_flowgraph.FlowgraphNode) { n.ObjectKey = "" },
		"bad hold":  func(n *s4wave_flowgraph.FlowgraphNode) { n.Parameters[ParamHold] = "soon" },
		"no source": func(n *s4wave_flowgraph.FlowgraphNode) { delete(n.Parameters, ParamSource) },
	} {
		bad := node.Node.CloneVT()
		change(bad)
		if _, err := (NodeType{}).Compile(context.Background(), &s4wave_flowgraph.PlacedFlowgraphNode{EngineID: "engine", Node: bad}); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
}
