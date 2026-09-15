package block_test

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// graphTestNode supplies stable IDs independent of insertion order.
type graphTestNode int64

func (n graphTestNode) ID() int64 { return int64(n) }

// graphTestEdge connects two test nodes.
type graphTestEdge [2]graphTestNode

func (e graphTestEdge) From() block.GraphNode { return e[0] }
func (e graphTestEdge) To() block.GraphNode   { return e[1] }

// TestBlockGraphMutation preserves replacement, bidirectional removal, stable
// topological order, and cycle detection across adjacency changes.
func TestBlockGraphMutation(t *testing.T) {
	g := block.NewBlockGraph()
	for _, edge := range []graphTestEdge{{8, 2}, {8, 3}, {2, 1}, {3, 1}, {8, 2}, {-1, 8}} {
		g.SetEdge(edge)
	}
	if got := len(g.Edges()); got != 5 {
		t.Fatalf("replacement left %d edges", got)
	}
	check := func(want []int64) {
		t.Helper()
		nodes, err := block.SortBlockGraph(g)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]int64, len(nodes))
		for i, node := range nodes {
			got[i] = node.ID()
		}
		if !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	check([]int64{-1, 8, 2, 3, 1})
	g.RemoveEdge(8, 2)
	g.RemoveEdge(8, 2)
	check([]int64{-1, 2, 8, 3, 1})
	g.RemoveNode(3)
	check([]int64{-1, 2, 1, 8})
	if len(g.Edges()) != 2 || len(g.From(8)) != 0 {
		t.Fatal("removed node retained an incoming or outgoing edge")
	}
	g.SetEdge(graphTestEdge{1, 2})
	if _, err := block.SortBlockGraph(g); err == nil {
		t.Fatal("accepted cyclic graph")
	}
}
