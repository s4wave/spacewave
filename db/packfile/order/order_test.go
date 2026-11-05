package order

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// testLists is a fake of the child lists a Volume keeps.
type testLists map[string][]*block.BlockRef

// GetChildren returns the children listed for ref.
func (l testLists) GetChildren(_ context.Context, ref *block.BlockRef) ([]*block.BlockRef, bool, error) {
	children, ok := l[refKey(ref)]
	return slices.Clone(children), ok, nil
}

func TestBlockRefsKeepsSubtreesTogether(t *testing.T) {
	// Create candidate block references for ordering.
	ctx := context.Background()
	rootA := testRef(t, "root-a")
	childA := testRef(t, "child-a")
	rootB := testRef(t, "root-b")
	stray := testRef(t, "stray")

	// List the child of one root.
	lists := testLists{refKey(rootA): {childA}}

	// Order the candidates using the lists.
	ordered, err := BlockRefs(ctx, lists, []*block.BlockRef{
		stray,
		childA,
		rootB,
		rootA,
	})
	if err != nil {
		t.Fatalf("BlockRefs: %v", err)
	}

	// Verify the child follows its parent at once.
	keys := refKeys(ordered)
	i := slices.Index(keys, refKey(rootA))
	if i < 0 || i+1 >= len(keys) || keys[i+1] != refKey(childA) || len(keys) != 4 {
		t.Fatalf("order = %v, want child-a right after root-a", keys)
	}
}

func TestBlockRefsFallbackIsStable(t *testing.T) {
	// Create candidate block references for ordering.
	ctx := context.Background()
	refs := []*block.BlockRef{
		testRef(t, "zeta"),
		testRef(t, "alpha"),
		testRef(t, "middle"),
	}

	// Order the candidates using the available reference graph.
	ordered, err := BlockRefs(ctx, nil, refs)
	if err != nil {
		t.Fatalf("BlockRefs: %v", err)
	}

	// Verify graph-free ordering follows sorted reference keys.
	keys := refKeys(refs)
	slices.Sort(keys)
	if got := refKeys(ordered); !slices.Equal(got, keys) {
		t.Fatalf("fallback order = %v, want %v", got, keys)
	}
}

func testRef(t *testing.T, body string) *block.BlockRef {
	t.Helper()
	h, err := hash.Sum(hash.RecommendedHashType, []byte(body))
	if err != nil {
		t.Fatalf("sum hash: %v", err)
	}
	return block.NewBlockRef(h)
}

func assertRefOrder(t *testing.T, got, want []*block.BlockRef) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d refs, want %d", len(got), len(want))
	}
	for i := range want {
		gotKey := got[i].GetHash().MarshalString()
		wantKey := want[i].GetHash().MarshalString()
		if gotKey != wantKey {
			t.Fatalf("ref %d = %s, want %s", i, gotKey, wantKey)
		}
	}
}

func refKeys(refs []*block.BlockRef) []string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, ref.GetHash().MarshalString())
	}
	return keys
}
