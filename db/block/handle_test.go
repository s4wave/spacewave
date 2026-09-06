package block

import "testing"

// TestSharedHandleReparenting preserves unrelated incoming references when a
// source switches between shared targets.
func TestSharedHandleReparenting(t *testing.T) {
	// Give one target many parents with small outgoing reference sets.
	_, root := NewTransaction(nil, nil, nil, nil)
	target := root.FollowRef(1, nil)
	parents := make([]*Cursor, 16)
	for i := range parents {
		parents[i] = root.FollowRef(uint32(i+2), nil)
		parents[i].SetRef(1, target)
	}
	if got := len(target.Parents()); got != len(parents)+1 {
		t.Fatalf("shared target has %d parents, want %d", got, len(parents)+1)
	}

	// Move one source while retaining the other incoming relationships.
	other := root.FollowRef(100, nil)
	parents[0].SetRef(1, other)
	if got := len(target.Parents()); got != len(parents) {
		t.Fatalf("old target has %d parents after reparenting, want %d", got, len(parents))
	}
	if got := len(other.Parents()); got != 2 {
		t.Fatalf("new target has %d parents, want 2", got)
	}
	for _, parent := range target.Parents() {
		if parent.pos == parents[0].pos {
			t.Fatal("old target retained the moved parent")
		}
	}

	// Repeating the assignment must not duplicate the incoming edge.
	parents[0].SetRef(1, other)
	if got := len(other.Parents()); got != 2 {
		t.Fatalf("repeated assignment left %d parents, want 2", got)
	}
}
