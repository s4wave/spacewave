package world

import (
	"context"
	"testing"
)

// readStubWorldState answers the reads ReadSet tracks with empty results.
type readStubWorldState struct {
	WorldState
}

// GetObject reports that no object exists.
func (readStubWorldState) GetObject(context.Context, string) (ObjectState, bool, error) {
	return nil, false, nil
}

// IterateObjects returns no iterator.
func (readStubWorldState) IterateObjects(context.Context, string, bool) ObjectIterator {
	return nil
}

// LookupGraphQuads returns no quads.
func (readStubWorldState) LookupGraphQuads(context.Context, GraphQuad, uint32) ([]GraphQuad, error) {
	return nil, nil
}

// TestReadSetTouched checks which ChangeSets touch the reads of a view.
func TestReadSetTouched(t *testing.T) {
	// Read key a, iterate prefix dir/, and look up (s, p, *).
	ctx := t.Context()
	reads := NewReadSet()
	ws := reads.Track(readStubWorldState{})
	obj, _, err := ws.GetObject(ctx, "a")
	ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	ws.IterateObjects(ctx, "dir/", false)
	if _, err := ws.LookupGraphQuads(ctx, NewGraphQuadWithKeys("s", "p", "", ""), 0); err != nil {
		t.Fatal(err)
	}

	// Compare each ChangeSet against the reads.
	cases := []struct {
		name    string
		changes *ChangeSet
		touched bool
	}{
		{"empty", &ChangeSet{}, false},
		{"other key", &ChangeSet{Keys: []string{"b"}}, false},
		{"other subject", &ChangeSet{Quads: []GraphQuad{NewGraphQuadWithKeys("s2", "p", "o", "")}}, false},
		{"read key", &ChangeSet{Keys: []string{"a"}}, true},
		{"create under prefix", &ChangeSet{Keys: []string{"dir/x"}}, true},
		{"matching quad", &ChangeSet{Quads: []GraphQuad{NewGraphQuadWithKeys("s", "p", "o", "")}}, true},
		{"rename into read key", &ChangeSet{Keys: []string{"c", "a"}}, true},
		{"pattern subject", &ChangeSet{Keys: []string{"s"}}, true},
		{"unknown", NewUnknownChangeSet(), true},
	}
	for _, c := range cases {
		if got := reads.Touched(c.changes); got != c.touched {
			t.Errorf("%s: touched = %v, want %v", c.name, got, c.touched)
		}
	}
}
