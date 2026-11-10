package kvtx_cayley_nodegraph

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/util/extsort"
)

// treeEntry is one key and value of a tree.
type treeEntry struct{ k, v []byte }

// readTree returns every key and value of tx in order.
func readTree(t *testing.T, ctx context.Context, tx kvtx.TxOps) []treeEntry {
	t.Helper()
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()
	var out []treeEntry
	for it.Next() {
		v, err := it.ValueCopy(nil)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, treeEntry{bytes.Clone(it.Key()), v})
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// buildTree builds a tree with budget from adds, run in the given order.
func buildTree(t *testing.T, budget int, adds []func(*Builder) error) ([]treeEntry, error) {
	t.Helper()
	sorter := extsort.New(t.TempDir(), budget)
	defer sorter.Close()
	b := NewBuilder(sorter)
	for _, add := range adds {
		if err := add(b); err != nil {
			return nil, err
		}
	}
	var out []treeEntry
	err := b.Write(t.Context(), func(k, v []byte) error {
		out = append(out, treeEntry{bytes.Clone(k), bytes.Clone(v)})
		return nil
	})
	return out, err
}

// TestBuilderMatchesIncrementalTree checks that building a tree from the nodes
// and quads of a tree written by ApplyDeltas gives the same keys and values, in
// any add order and at any sort budget.
func TestBuilderMatchesIncrementalTree(t *testing.T) {
	// Write a graph with shared nodes, several predicates and some labels.
	ctx := context.Background()
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var deltas []graph.Delta
	for i := range 300 {
		q := quad.MakeIRI(fmt.Sprintf("s%d", i%17), fmt.Sprintf("p%d", i%5), fmt.Sprintf("o%d", i), "")
		if i%7 == 0 {
			q.Label = quad.IRI(fmt.Sprintf("l%d", i%3))
		}
		deltas = append(deltas, graph.Delta{Quad: q, Action: graph.Add})
	}
	if err := NewQuadStore(tx).ApplyDeltas(ctx, deltas, graph.IgnoreOpts{}); err != nil {
		t.Fatal(err)
	}
	want := readTree(t, ctx, tx)

	// Add the tree's nodes and quads back through a Builder.
	var adds []func(*Builder) error
	for _, e := range want {
		id, tag, ok := parseNodeKey(e.k)
		switch {
		case ok && tag == tagMeta:
			meta := &NodeMeta{}
			if err := meta.UnmarshalVT(e.v); err != nil {
				t.Fatal(err)
			}
			adds = append(adds, func(b *Builder) error { return b.AddNode(id, meta.GetValue()) })
		case ok && tag == tagOut:
			q, _ := parseEdgeKey(e.k)
			adds = append(adds, func(b *Builder) error { return b.AddQuad(q.s, q.p, q.o, q.l) })
		}
	}
	r := rand.New(rand.NewPCG(3, 4))
	for _, budget := range []int{1 << 20, 1} {
		r.Shuffle(len(adds), func(i, j int) { adds[i], adds[j] = adds[j], adds[i] })
		got, err := buildTree(t, budget, adds)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.EqualFunc(got, want, func(a, b treeEntry) bool {
			return bytes.Equal(a.k, b.k) && bytes.Equal(a.v, b.v)
		}) {
			t.Fatalf("budget %d: built tree differs from the incremental tree", budget)
		}
	}
}

// TestBuilderRejectsBadGraph checks that a value added twice and a quad naming
// a node never added fail the build.
func TestBuilderRejectsBadGraph(t *testing.T) {
	v := func(b *Builder, id uint64, s string) error { return b.AddNode(id, pquads.MakeValue(quad.IRI(s))) }
	cases := map[string][]func(*Builder) error{
		"value twice": {
			func(b *Builder) error { return v(b, 1, "a") },
			func(b *Builder) error { return v(b, 2, "a") },
		},
		"missing predicate": {
			func(b *Builder) error { return v(b, 1, "a") },
			func(b *Builder) error { return b.AddQuad(1, 2, 1, 0) },
		},
	}
	for name, adds := range cases {
		if _, err := buildTree(t, 1, adds); err == nil {
			t.Fatalf("%s: build succeeded", name)
		}
	}
}

// TestBuilderNextIDPastLargest checks that a built tree allocates past its
// largest node, the case of a regeneration whose source freed IDs below and
// above the nodes it keeps.
func TestBuilderNextIDPastLargest(t *testing.T) {
	// Build nodes 3 and 9 and the quad joining them through predicate 5.
	ctx := context.Background()
	add := func(id uint64, s string) func(*Builder) error {
		return func(b *Builder) error { return b.AddNode(id, pquads.MakeValue(quad.IRI(s))) }
	}
	built, err := buildTree(t, 1, []func(*Builder) error{
		add(3, "a"), add(5, "p"), add(9, "b"),
		func(b *Builder) error { return b.AddQuad(3, 5, 9, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, e := range built {
		if err := tx.Set(ctx, e.k, e.v); err != nil {
			t.Fatal(err)
		}
	}

	// A new node takes ID 10.
	d := graph.Delta{Quad: quad.MakeIRI("a", "p", "c", ""), Action: graph.Add}
	if err := NewQuadStore(tx).ApplyDeltas(ctx, []graph.Delta{d}, graph.IgnoreOpts{}); err != nil {
		t.Fatal(err)
	}
	v, _, err := tx.Get(ctx, valueKey(quad.IRI("c").String()))
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := decodeID(v); id != 10 {
		t.Fatalf("new node has ID %d, want 10", id)
	}
}
