package kvtx_cayley

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/kvtx"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// Hide only the optional write capability to compare the production scalar and
// coalesced graph paths over otherwise identical Okra/TxStore implementations.
type scalarGraphStore struct{ kvtx.Store }

func (s scalarGraphStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return struct{ kvtx.Tx }{tx}, nil
}

func TestCayleyWriteBatchMatchesScalarStorage(t *testing.T) {
	// Prepare the shared block store and graph fixture type.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	type fixture struct {
		btx   *block.Transaction
		tree  *okra.Tx
		graph *cayley.Handle
	}

	// Open scalar or batched Cayley graphs over identical Okra transactions.
	open := func(ref *block.BlockRef, batch bool) fixture {
		// Open the block transaction and its Okra tree.
		btx, root := block.NewTransaction(store, nil, ref, nil)
		tree, err := okra.NewTxWithInlineValues(ctx, root, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Select the graph store write capability.
		var objStore kvtx.Store = kvtx.NewTxStore(tree)
		if !batch {
			objStore = scalarGraphStore{objStore}
		}

		// Open the Cayley graph with the default index layout.
		hd, err := NewGraph(ctx, objStore, graph.Options{cayley_kv.OptAssumeDefaultIdx: true})
		if err != nil {
			t.Fatal(err)
		}

		return fixture{btx, tree, hd}
	}

	// Acquire both graph fixtures and release them after the comparison.
	batch, scalar := open(nil, true), open(nil, false)
	closeFixture := func(f fixture) { _ = f.graph.Close(); f.tree.Discard() }
	defer func() { closeFixture(batch); closeFixture(scalar) }()

	// Compare the persisted roots of the scalar and batched graph fixtures.
	compare := func() {
		// Persist the batched graph transaction.
		br, _, err := batch.btx.Write(ctx, false)
		if err != nil {
			t.Fatal(err)
		}

		// Persist the scalar graph transaction.
		sr, _, err := scalar.btx.Write(ctx, false)
		if err != nil {
			t.Fatal(err)
		}

		// Require both graph transactions to produce the same root.
		if !br.EqualsRef(sr) {
			t.Fatal("batched graph has different persisted Okra root")
		}
	}

	// Prepare quads that share subjects across graph index entries.
	// Fresh and reopened stores, shared nodes, duplicate additions, deletions,
	// index scans and mutations after scans all use actual Cayley implementations.
	quads := make([]quad.Quad, 192)
	for i := range quads {
		quads[i] = quad.Make(fmt.Sprintf("subject-%03d", i%17), "next", fmt.Sprintf("object-%03d", i), nil)
	}

	// Add the quad fixture to both graph stores in matching batches.
	for _, f := range []fixture{batch, scalar} {
		for i := 0; i < len(quads); i += 32 {
			if err := f.graph.AddQuadSet(ctx, quads[i:i+32]); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Require the initial graph batches to persist identically.
	compare()

	// Compare graph mutation and indexed reads over both write paths.
	for _, f := range []fixture{batch, scalar} {
		// Remove every third quad from the graph fixture.
		for i, q := range quads {
			if i%3 == 0 {
				if err := f.graph.RemoveQuad(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
		}

		// Restore the removed quads and exercise duplicate insertion.
		if err := f.graph.AddQuadSet(ctx, quads); err != nil {
			t.Fatal(err)
		}

		// Read the graph index for the selected subject.
		var got []string
		err := cayley.StartPath(f.graph, quad.String("subject-003")).Out(quad.String("next")).Iterate(ctx).EachValue(ctx, nil, func(v quad.Value) error { got = append(got, quad.NativeOf(v).(string)); return nil })
		if err != nil {
			t.Fatal(err)
		}

		// Build the expected objects for the selected subject.
		var want []string
		for i := 3; i < len(quads); i += 17 {
			want = append(want, fmt.Sprintf("object-%03d", i))
		}

		// Require the graph index to contain exactly the expected objects.
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("index results: %v != %v", got, want)
		}
	}

	// Require both mutated graph fixtures to persist identically.
	compare()

	// Persist the graph roots for reopening.
	br, _, err := batch.btx.Write(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	sr, _, err := scalar.btx.Write(ctx, false)
	if err != nil {
		t.Fatal(err)
	}

	// Reopen both graph fixtures from their persisted roots.
	closeFixture(batch)
	closeFixture(scalar)
	batch, scalar = open(br, true), open(sr, false)

	// Mutate each reopened graph through deletions and a new quad.
	for _, f := range []fixture{batch, scalar} {
		// Remove the first batch of quads from the reopened graph.
		for i := range 32 {
			if err := f.graph.RemoveQuad(ctx, quads[i]); err != nil {
				t.Fatal(err)
			}
		}

		// Add a new subject and object to the reopened graph.
		if err := f.graph.AddQuad(ctx, quad.Make("new-subject", "next", "new-object", nil)); err != nil {
			t.Fatal(err)
		}
	}

	// Require the reopened graph mutations to persist identically.
	compare()
}
