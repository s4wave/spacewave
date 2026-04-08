package block_gc

import (
	"context"
	"slices"
	"testing"
)

// mockSweepTarget records which blocks and objects were deleted.
type mockSweepTarget struct {
	deletedBlocks  []string
	deletedObjects []string
}

func (m *mockSweepTarget) DeleteBlock(_ context.Context, iri string) error {
	m.deletedBlocks = append(m.deletedBlocks, iri)
	return nil
}

func (m *mockSweepTarget) DeleteObject(_ context.Context, iri string) error {
	m.deletedObjects = append(m.deletedObjects, iri)
	return nil
}

func noopReplay(_ context.Context, _ CollectorGraph) (int, error) {
	return 0, nil
}

func noopSTW() (func(), error) {
	return func() {}, nil
}

func TestSweepCycleBasic(t *testing.T) {
	g := newMockGraph()
	target := &mockSweepTarget{}

	// root -> live -> child
	// orphan (unreachable)
	g.addRoot("root")
	g.addEdge("root", "live")
	g.addEdge("live", "child")
	g.addNode("orphan")

	cfg := SweepConfig{
		Graph:      g,
		Target:     target,
		ReplayWAL:  noopReplay,
		AcquireSTW: noopSTW,
	}

	result, err := SweepCycle(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.SweepCandidates != 1 {
		t.Errorf("SweepCandidates = %d, want 1", result.SweepCandidates)
	}
	if result.Swept != 1 {
		t.Errorf("Swept = %d, want 1", result.Swept)
	}
	if result.Rescued != 0 {
		t.Errorf("Rescued = %d, want 0", result.Rescued)
	}
}

func TestSweepCycleTransitiveRescue(t *testing.T) {
	g := newMockGraph()
	target := &mockSweepTarget{}

	// root -> a
	// orphan-parent -> orphan-child (both unreachable)
	g.addRoot("root")
	g.addEdge("root", "a")
	g.addNode("orphan-parent")
	g.addNode("orphan-child")
	g.addEdge("orphan-parent", "orphan-child")

	// Phase 2 WAL replay will add an edge making orphan-parent reachable.
	phase2Called := false
	replay := func(_ context.Context, graph CollectorGraph) (int, error) {
		if !phase2Called {
			phase2Called = true
			return 0, nil
		}
		// Phase 2: root now references orphan-parent.
		return 1, graph.AddRef(context.Background(), "root", "orphan-parent")
	}

	cfg := SweepConfig{
		Graph:      g,
		Target:     target,
		ReplayWAL:  replay,
		AcquireSTW: noopSTW,
	}

	result, err := SweepCycle(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Both orphan-parent and orphan-child should be rescued.
	if result.Rescued != 2 {
		t.Errorf("Rescued = %d, want 2", result.Rescued)
	}
	if result.Swept != 0 {
		t.Errorf("Swept = %d, want 0", result.Swept)
	}
}

func TestSweepCycleGraphCleanup(t *testing.T) {
	g := newMockGraph()
	target := &mockSweepTarget{}

	// root -> live
	// orphan -> child (orphan is unreachable, child is in inventory via addEdge)
	g.addRoot("root")
	g.addEdge("root", "live")
	g.addEdge("orphan", "child")

	cfg := SweepConfig{
		Graph:      g,
		Target:     target,
		ReplayWAL:  noopReplay,
		AcquireSTW: noopSTW,
	}

	result, err := SweepCycle(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Both orphan and child are unreachable (orphan has no incoming root
	// edge, and child is only reachable from orphan). Both are swept.
	if result.Swept != 2 {
		t.Errorf("Swept = %d, want 2", result.Swept)
	}

	// After sweep, orphan's outgoing edges should be cleaned up.
	out, _ := g.GetOutgoingRefs(context.Background(), "orphan")
	if len(out) != 0 {
		t.Errorf("orphan still has outgoing refs after sweep: %v", out)
	}

	// Verify root and live are untouched.
	outRoot, _ := g.GetOutgoingRefs(context.Background(), "root")
	if !slices.Contains(outRoot, "live") {
		t.Errorf("root->live edge was incorrectly removed")
	}
}

func TestSweepCycleObjectDelete(t *testing.T) {
	g := newMockGraph()
	target := &mockSweepTarget{}

	g.addRoot("root")
	g.addNode("object:stale-key")

	cfg := SweepConfig{
		Graph:      g,
		Target:     target,
		ReplayWAL:  noopReplay,
		AcquireSTW: noopSTW,
	}

	result, err := SweepCycle(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Swept != 1 {
		t.Errorf("Swept = %d, want 1", result.Swept)
	}
	if !slices.Contains(target.deletedObjects, "object:stale-key") {
		t.Errorf("object not deleted: %v", target.deletedObjects)
	}
}
