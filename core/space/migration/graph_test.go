package space_migration

import "testing"

func TestMappedGraphReferencesPreservePredicatesAndMapLabels(t *testing.T) {
	// Define destination mappings for graph object and label identities.
	mapping := NewIdentityMap()
	mapping.GraphIRIs["<source/object>"] = "<destination/object>"
	mapping.GraphIRIs["<source/label>"] = "<destination/label>"
	mapping.GraphIRIs["source/object"] = "destination/object"
	mapping.GraphIRIs["source/label"] = "destination/label"

	// Remap graph references while preserving predicates and external objects.
	got := mappedGraphReferences(&ObjectDescriptor{GraphReferences: []string{
		"<source/object>", "<predicate>", "<external/object>", "<source/label>",
	}}, mapping)

	// Verify the mapped graph retains its shape and expected values.
	want := []string{"<destination/object>", "<predicate>", "<external/object>", "<destination/label>"}
	if len(got) != len(want) {
		t.Fatalf("mapped graph length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mapped graph[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
