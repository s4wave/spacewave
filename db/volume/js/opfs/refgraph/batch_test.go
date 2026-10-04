package refgraph

import (
	"context"
	"errors"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestApplyRefBatchOwnershipSemantics covers complete ownership transitions.
func TestApplyRefBatchOwnershipSemantics(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		seed    []block_gc.RefEdge
		adds    []block_gc.RefEdge
		removes []block_gc.RefEdge
		want    []block_gc.RefEdge
		reject  []block_gc.RefEdge
	}{
		{
			name:    "ownership transfer",
			seed:    []block_gc.RefEdge{{Subject: "old", Object: "object"}},
			adds:    []block_gc.RefEdge{{Subject: "new", Object: "object"}},
			removes: []block_gc.RefEdge{{Subject: "old", Object: "object"}},
			want:    []block_gc.RefEdge{{Subject: "new", Object: "object"}},
			reject:  []block_gc.RefEdge{{Subject: "old", Object: "object"}, {Subject: block_gc.NodeUnreferenced, Object: "object"}},
		},
		{
			name: "duplicate add",
			adds: []block_gc.RefEdge{{Subject: "owner", Object: "object"}, {Subject: "owner", Object: "object"}},
			want: []block_gc.RefEdge{{Subject: "owner", Object: "object"}},
		},
		{
			name:    "absent removal",
			removes: []block_gc.RefEdge{{Subject: "missing", Object: "object"}},
			reject:  []block_gc.RefEdge{{Subject: block_gc.NodeUnreferenced, Object: "object"}},
		},
		{
			name:    "final owner orphan mark",
			seed:    []block_gc.RefEdge{{Subject: "owner", Object: "object"}},
			removes: []block_gc.RefEdge{{Subject: "owner", Object: "object"}},
			want:    []block_gc.RefEdge{{Subject: block_gc.NodeUnreferenced, Object: "object"}},
			reject:  []block_gc.RefEdge{{Subject: "owner", Object: "object"}},
		},
		{
			name:    "permanent root is not orphaned",
			seed:    []block_gc.RefEdge{{Subject: "owner", Object: block_gc.NodeGCRoot}},
			removes: []block_gc.RefEdge{{Subject: "owner", Object: block_gc.NodeGCRoot}},
			reject:  []block_gc.RefEdge{{Subject: block_gc.NodeUnreferenced, Object: block_gc.NodeGCRoot}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Start a graph and apply the case's ownership transition.
			graph := NewGraph(store_kvtx_inmem.NewStore())
			if err := graph.ApplyRefBatch(ctx, test.seed, nil); err != nil {
				t.Fatal(err)
			}
			if err := graph.ApplyRefBatch(ctx, test.adds, test.removes); err != nil {
				t.Fatal(err)
			}

			// Verify every expected edge is present.
			for _, edge := range test.want {
				assertEdge(t, graph, edge, true)
			}

			// Verify every forbidden edge is absent.
			for _, edge := range test.reject {
				assertEdge(t, graph, edge, false)
			}
		})
	}
}

// TestRejectedTransactionLeavesNeitherHalfEdge proves atomic edge insertion.
func TestRejectedTransactionLeavesNeitherHalfEdge(t *testing.T) {
	// Reject an edge write and confirm the transaction rolls back both indexes.
	ctx := context.Background()
	graph := NewGraph(store_kvtx_inmem.NewStore())
	rejected := errors.New("reject transaction")
	edge := block_gc.RefEdge{Subject: "owner", Object: "object"}

	// Roll back after staging both graph directions.
	err := kvtx.RunTransaction(
		ctx,
		true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return graph.store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := graph.applyRefBatchTx(ctx, tx, []block_gc.RefEdge{edge}, nil); err != nil {
				return err
			}
			return rejected
		},
	)
	if !errors.Is(err, rejected) {
		t.Fatalf("error = %v, want %v", err, rejected)
	}

	// Confirm rollback removed the forward, reverse, and node records.
	assertEdge(t, graph, edge, false)
	assertRecord(t, graph, graphKey('n', edge.Subject), false)
	assertRecord(t, graph, graphKey('n', edge.Object), false)
}

// assertEdge checks that both stored directions agree on edge existence.
func assertEdge(t testing.TB, graph *Graph, edge block_gc.RefEdge, want bool) {
	t.Helper()
	assertRecord(t, graph, graphKey('f', edge.Subject, edge.Object), want)
	assertRecord(t, graph, graphKey('i', edge.Object, edge.Subject), want)
}

// assertRecord checks one graph record through a fresh read transaction.
func assertRecord(t testing.TB, graph *Graph, key []byte, want bool) {
	// Read the record in a fresh transaction and compare its existence.
	t.Helper()
	tx, err := graph.store.NewTransaction(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Check whether the requested graph record exists.
	exists, err := tx.Exists(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if exists != want {
		t.Fatalf("record %x exists = %v, want %v", key, exists, want)
	}
}
