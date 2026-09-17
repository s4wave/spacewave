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
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	type fixture struct {
		btx   *block.Transaction
		tree  *okra.Tx
		graph *cayley.Handle
	}
	open := func(ref *block.BlockRef, batch bool) fixture {
		btx, root := block.NewTransaction(store, nil, ref, nil)
		tree, err := okra.NewTxWithInlineValues(ctx, root, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		var objStore kvtx.Store = kvtx.NewTxStore(tree)
		if !batch {
			objStore = scalarGraphStore{objStore}
		}
		hd, err := NewGraph(ctx, objStore, graph.Options{cayley_kv.OptAssumeDefaultIdx: true})
		if err != nil {
			t.Fatal(err)
		}
		return fixture{btx, tree, hd}
	}
	batch, scalar := open(nil, true), open(nil, false)
	closeFixture := func(f fixture) { _ = f.graph.Close(); f.tree.Discard() }
	defer func() { closeFixture(batch); closeFixture(scalar) }()
	compare := func() {
		br, _, err := batch.btx.Write(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		sr, _, err := scalar.btx.Write(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if !br.EqualsRef(sr) {
			t.Fatal("batched graph has different persisted Okra root")
		}
	}
	// Fresh and reopened stores, shared nodes, duplicate additions, deletions,
	// index scans and mutations after scans all use actual Cayley implementations.
	quads := make([]quad.Quad, 192)
	for i := range quads {
		quads[i] = quad.Make(fmt.Sprintf("subject-%03d", i%17), "next", fmt.Sprintf("object-%03d", i), nil)
	}
	for _, f := range []fixture{batch, scalar} {
		for i := 0; i < len(quads); i += 32 {
			if err := f.graph.AddQuadSet(ctx, quads[i:i+32]); err != nil {
				t.Fatal(err)
			}
		}
	}
	compare()
	for _, f := range []fixture{batch, scalar} {
		for i, q := range quads {
			if i%3 == 0 {
				if err := f.graph.RemoveQuad(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := f.graph.AddQuadSet(ctx, quads); err != nil {
			t.Fatal(err)
		}
		var got []string
		err := cayley.StartPath(f.graph, quad.String("subject-003")).Out(quad.String("next")).Iterate(ctx).EachValue(ctx, nil, func(v quad.Value) error { got = append(got, quad.NativeOf(v).(string)); return nil })
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for i := 3; i < len(quads); i += 17 {
			want = append(want, fmt.Sprintf("object-%03d", i))
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("index results: %v != %v", got, want)
		}
	}
	compare()
	br, _, err := batch.btx.Write(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	sr, _, err := scalar.btx.Write(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	closeFixture(batch)
	closeFixture(scalar)
	batch, scalar = open(br, true), open(sr, false)
	for _, f := range []fixture{batch, scalar} {
		for i := range 32 {
			if err := f.graph.RemoveQuad(ctx, quads[i]); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.graph.AddQuad(ctx, quad.Make("new-subject", "next", "new-object", nil)); err != nil {
			t.Fatal(err)
		}
	}
	compare()
}
