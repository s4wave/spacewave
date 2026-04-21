package block_gc

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// mockCollectorGraph is a simple in-memory CollectorGraph for testing.
type mockCollectorGraph struct {
	nodes         []string
	roots         []string
	edges         map[string][]string // subject -> []object
	closeFn       func() error
	removeRootErr error
	removeNodeErr error
}

func newMockGraph() *mockCollectorGraph {
	return &mockCollectorGraph{
		edges: make(map[string][]string),
	}
}

func (m *mockCollectorGraph) addNode(iri string) {
	if !slices.Contains(m.nodes, iri) {
		m.nodes = append(m.nodes, iri)
	}
}

func (m *mockCollectorGraph) addEdge(subj, obj string) {
	m.addNode(subj)
	m.addNode(obj)
	m.edges[subj] = append(m.edges[subj], obj)
}

func (m *mockCollectorGraph) addRoot(iri string) {
	m.addNode(iri)
	if !slices.Contains(m.roots, iri) {
		m.roots = append(m.roots, iri)
	}
}

func (m *mockCollectorGraph) IterateNodes(_ context.Context) ([]string, error) {
	return slices.Clone(m.nodes), nil
}

func (m *mockCollectorGraph) GetRootNodes(_ context.Context) ([]string, error) {
	return slices.Clone(m.roots), nil
}

func (m *mockCollectorGraph) GetOutgoingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(m.edges[node]), nil
}

func (m *mockCollectorGraph) GetIncomingRefs(_ context.Context, node string) ([]string, error) {
	var sources []string
	for s, targets := range m.edges {
		if slices.Contains(targets, node) {
			sources = append(sources, s)
		}
	}
	return sources, nil
}

func (m *mockCollectorGraph) HasIncomingRefs(_ context.Context, node string) (bool, error) {
	for s, targets := range m.edges {
		if s == NodeUnreferenced {
			continue
		}
		if slices.Contains(targets, node) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockCollectorGraph) HasIncomingRefsExcluding(
	_ context.Context,
	node string,
	excluded ...string,
) (bool, error) {
	excludedSet := make(map[string]struct{}, len(excluded)+1)
	excludedSet[NodeUnreferenced] = struct{}{}
	for _, src := range excluded {
		excludedSet[src] = struct{}{}
	}
	for s, targets := range m.edges {
		if _, ok := excludedSet[s]; ok {
			continue
		}
		if slices.Contains(targets, node) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockCollectorGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return m.GetOutgoingRefs(ctx, NodeUnreferenced)
}

func (m *mockCollectorGraph) AddRef(_ context.Context, subj, obj string) error {
	m.addEdge(subj, obj)
	return nil
}

func (m *mockCollectorGraph) RemoveRef(_ context.Context, subj, obj string) error {
	targets := m.edges[subj]
	idx := slices.Index(targets, obj)
	if idx >= 0 {
		m.edges[subj] = slices.Delete(targets, idx, idx+1)
	}
	return nil
}

func (m *mockCollectorGraph) ApplyRefBatch(_ context.Context, adds, removes []RefEdge) error {
	for _, e := range adds {
		m.addEdge(e.Subject, e.Object)
	}
	for _, e := range removes {
		targets := m.edges[e.Subject]
		idx := slices.Index(targets, e.Object)
		if idx >= 0 {
			m.edges[e.Subject] = slices.Delete(targets, idx, idx+1)
		}
	}
	return nil
}

func (m *mockCollectorGraph) RemoveNodeRefs(_ context.Context, node string, _ bool) ([]string, error) {
	targets := slices.Clone(m.edges[node])
	delete(m.edges, node)
	return targets, nil
}

func (m *mockCollectorGraph) AddBlockRef(_ context.Context, _, _ *block.BlockRef) error {
	return nil
}

func (m *mockCollectorGraph) AddObjectRoot(_ context.Context, _ string, _ *block.BlockRef) error {
	return nil
}

func (m *mockCollectorGraph) RemoveObjectRoot(_ context.Context, _ string, _ *block.BlockRef) error {
	return nil
}

func (m *mockCollectorGraph) RemoveRoot(_ context.Context, iri string) error {
	if m.removeRootErr != nil {
		return m.removeRootErr
	}
	idx := slices.Index(m.roots, iri)
	if idx >= 0 {
		m.roots = slices.Delete(m.roots, idx, idx+1)
	}
	return nil
}

func (m *mockCollectorGraph) RemoveNode(_ context.Context, iri string) error {
	if m.removeNodeErr != nil {
		return m.removeNodeErr
	}
	idx := slices.Index(m.nodes, iri)
	if idx >= 0 {
		m.nodes = slices.Delete(m.nodes, idx, idx+1)
	}
	return nil
}

func (m *mockCollectorGraph) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// TestMarkerBasicReachability tests that reachable nodes are black and
// unreachable nodes are white (sweep candidates).
func TestMarkerBasicReachability(t *testing.T) {
	g := newMockGraph()

	// Build graph:
	//   root1 -> a -> b -> c
	//   root2 -> d
	//   orphan (no incoming edges, not a root)
	g.addRoot("root1")
	g.addRoot("root2")
	g.addEdge("root1", "a")
	g.addEdge("a", "b")
	g.addEdge("b", "c")
	g.addEdge("root2", "d")
	g.addNode("orphan")

	marker := NewMarker(g)
	candidates, colors, err := marker.Mark(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// root1, a, b, c, root2, d should be black.
	for _, n := range []string{"root1", "a", "b", "c", "root2", "d"} {
		if colors[n] != Black {
			t.Errorf("node %q color = %d, want Black", n, colors[n])
		}
	}

	// orphan should be white and in sweep candidates.
	if colors["orphan"] != White {
		t.Errorf("orphan color = %d, want White", colors["orphan"])
	}
	if !slices.Contains(candidates, "orphan") {
		t.Errorf("orphan not in sweep candidates: %v", candidates)
	}
	if len(candidates) != 1 {
		t.Errorf("sweep candidates = %v, want [orphan]", candidates)
	}
}

// TestMarkerDanglingEdge tests that edges pointing to nodes not in the
// inventory are skipped (dangling from interrupted sweeps).
func TestMarkerDanglingEdge(t *testing.T) {
	g := newMockGraph()
	g.addRoot("root")
	g.addEdge("root", "live")
	// Add an edge to a node NOT in the inventory (simulates dangling ref).
	g.edges["live"] = append(g.edges["live"], "deleted-node")

	marker := NewMarker(g)
	candidates, colors, err := marker.Mark(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if colors["root"] != Black || colors["live"] != Black {
		t.Errorf("reachable nodes not black: root=%d live=%d", colors["root"], colors["live"])
	}
	if len(candidates) != 0 {
		t.Errorf("unexpected sweep candidates: %v", candidates)
	}
}

// TestMarkerEmptyGraph tests marking an empty graph.
func TestMarkerEmptyGraph(t *testing.T) {
	g := newMockGraph()
	marker := NewMarker(g)
	candidates, _, err := marker.Mark(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("empty graph produced candidates: %v", candidates)
	}
}

// TestMarkerPermanentRoots tests that gcroot and unreferenced are
// treated as roots when present in the inventory.
func TestMarkerPermanentRoots(t *testing.T) {
	g := newMockGraph()
	g.addNode(NodeGCRoot)
	g.addNode(NodeUnreferenced)
	g.addEdge(NodeGCRoot, "managed")
	g.addEdge(NodeUnreferenced, "orphan-tracked")

	marker := NewMarker(g)
	candidates, colors, err := marker.Mark(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if colors[NodeGCRoot] != Black {
		t.Errorf("gcroot not black")
	}
	if colors["managed"] != Black {
		t.Errorf("managed not black")
	}
	if colors[NodeUnreferenced] != Black {
		t.Errorf("unreferenced not black")
	}
	if colors["orphan-tracked"] != Black {
		t.Errorf("orphan-tracked not black (reachable via unreferenced)")
	}
	if len(candidates) != 0 {
		t.Errorf("unexpected sweep candidates: %v", candidates)
	}
}
