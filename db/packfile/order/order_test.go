package order

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/net/hash"
)

type testRefGraph struct {
	out map[string][]string
	in  map[string][]string
}

func newTestRefGraph() *testRefGraph {
	return &testRefGraph{
		out: make(map[string][]string),
		in:  make(map[string][]string),
	}
}

func (g *testRefGraph) add(subject, object string) {
	g.out[subject] = append(g.out[subject], object)
	g.in[object] = append(g.in[object], subject)
}

func (g *testRefGraph) GetOutgoingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.out[node]), nil
}

func (g *testRefGraph) GetIncomingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.in[node]), nil
}

func TestBlockRefsWalksObjectRoots(t *testing.T) {
	ctx := context.Background()
	rootA := testRef(t, "root-a")
	childA := testRef(t, "child-a")
	rootB := testRef(t, "root-b")
	stray := testRef(t, "stray")

	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("object-b"), block_gc.BlockIRI(rootB))
	graph.add(block_gc.ObjectIRI("object-a"), block_gc.BlockIRI(rootA))
	graph.add(block_gc.BlockIRI(rootA), block_gc.BlockIRI(childA))

	ordered, err := BlockRefs(ctx, graph, []*block.BlockRef{
		stray,
		childA,
		rootB,
		rootA,
	})
	if err != nil {
		t.Fatalf("BlockRefs: %v", err)
	}

	assertRefOrder(t, ordered, []*block.BlockRef{rootA, childA, rootB, stray})
}

func TestBlockRefsFallbackIsStable(t *testing.T) {
	ctx := context.Background()
	refs := []*block.BlockRef{
		testRef(t, "zeta"),
		testRef(t, "alpha"),
		testRef(t, "middle"),
	}

	ordered, err := BlockRefs(ctx, nil, refs)
	if err != nil {
		t.Fatalf("BlockRefs: %v", err)
	}

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
