package block

import (
	"bytes"
	"slices"
	"testing"
)

// TestDeadlistCanonicalLists preserves multiplicity and input slices while
// producing identical physical records for equivalent unordered field values.
func TestDeadlistCanonicalLists(t *testing.T) {
	// Give every field enough repeated references to cross leaf boundaries.
	a, err := BuildBlockRef([]byte("canonical-a"), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildBlockRef([]byte("canonical-b"), nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]*BlockRef, MaxDeadlistPartRefs+3)
	for i := range refs {
		refs[i] = a
		if i%3 == 0 {
			refs[i] = b
		}
	}
	original := slices.Clone(refs)
	reverse := slices.Clone(refs)
	slices.Reverse(reverse)
	first := &Replacements{Replaced: refs, Removed: refs, Kept: refs, Held: refs, Existed: refs, Dropped: refs, Found: refs, FallbackHeld: refs}
	second := &Replacements{Replaced: reverse, Removed: reverse, Kept: reverse, Held: reverse, Existed: reverse, Dropped: reverse, Found: reverse, FallbackHeld: reverse}

	// Equivalent lists must produce the same header and every physical payload.
	entries, root, err := EncodeDeadlist(1, a, b, first)
	if err != nil {
		t.Fatal(err)
	}
	other, otherRoot, err := EncodeDeadlist(1, a, b, second)
	if err != nil {
		t.Fatal(err)
	}
	if !root.EqualVT(otherRoot) || len(entries) != len(other) {
		t.Fatal("list order changed the physical record")
	}
	for i, entry := range entries {
		if !bytes.Equal(entry.Data, other[i].Data) {
			t.Fatal("list order changed a physical payload")
		}
	}

	// Count logical leaf occurrences through the header, including reused leaves.
	parts := make(map[string]*Deadlist, len(entries))
	for _, entry := range entries {
		part := &Deadlist{}
		if err := part.UnmarshalVT(entry.Data); err != nil {
			t.Fatal(err)
		}
		parts[entry.Ref.MarshalString()] = part
	}
	count := 0
	for _, ref := range parts[root.MarshalString()].Parts {
		part := parts[ref.MarshalString()]
		count += len(part.Replaced) + len(part.Removed) + len(part.Kept) + len(part.Held) + len(part.Existed) + len(part.Dropped) + len(part.Found) + len(part.FallbackHeld)
	}
	if count != 8*len(refs) {
		t.Fatalf("encoded occurrences=%d, want%d", count, 8*len(refs))
	}

	// Encoding must preserve caller ownership of the sealed input slices.
	if !slices.Equal(refs, original) {
		t.Fatal("encoding changed its input lists")
	}
}
