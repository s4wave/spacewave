package writer

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// TestWalkAccessOrder checks that siblings are yielded together before their
// contents, that a shared block is yielded once and that a missing block is
// skipped with its refs.
func TestWalkAccessOrder(t *testing.T) {
	// Build a directory "root" with children "a" and "b". "a" holds chunks
	// "a1" and "a2", "b" holds "b1" and the shared "a2", and "gone" is missing.
	tree := map[string][]string{
		"root": {"a", "b", "gone"},
		"a":    {"a1", "a2"},
		"b":    {"b1", "a2"},
		"a1":   nil,
		"a2":   nil,
		"b1":   nil,
	}
	refs := make(map[string]*block.BlockRef)
	names := make(map[string]string)
	for name := range tree {
		refs[name] = testRef(t, name)
		names[refs[name].GetHash().MarshalString()] = name
	}
	refs["gone"] = testRef(t, "gone")
	read := func(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
		// Report a block outside the tree as missing.
		name := names[ref.GetHash().MarshalString()]
		if name == "" {
			return nil, nil
		}

		// Return the block with its children as refs.
		stored := &block.StoredBlock{Data: []byte(name), RefsSource: block.RefsAsserted}
		for _, child := range tree[name] {
			stored.Refs = append(stored.Refs, refs[child])
		}
		return stored, nil
	}

	// Walk the tree and compare the order.
	iter := WalkAccessOrder(context.Background(), []*block.BlockRef{refs["root"]}, read)
	var got []string
	for {
		h, stored, err := iter()
		if err != nil {
			t.Fatal(err)
		}
		if h == nil {
			break
		}
		got = append(got, string(stored.Data))
	}
	want := []string{"root", "a", "b", "a1", "a2", "b1"}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// testRef returns the ref of the block whose data is name.
func testRef(t *testing.T, name string) *block.BlockRef {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(name))
	if err != nil {
		t.Fatal(err)
	}
	return block.NewBlockRef(h)
}
