package block_gc

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestApplyRefBatchOwnershipSemantics covers complete ownership transitions.
func TestApplyRefBatchOwnershipSemantics(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		seed    []RefEdge
		adds    []RefEdge
		removes []RefEdge
		want    []RefEdge
		reject  []RefEdge
	}{
		{
			name:    "ownership transfer",
			seed:    []RefEdge{{Subject: "old", Object: "object"}},
			adds:    []RefEdge{{Subject: "new", Object: "object"}},
			removes: []RefEdge{{Subject: "old", Object: "object"}},
			want:    []RefEdge{{Subject: "new", Object: "object"}},
			reject:  []RefEdge{{Subject: "old", Object: "object"}, {Subject: NodeUnreferenced, Object: "object"}},
		},
		{
			name: "duplicate add",
			adds: []RefEdge{{Subject: "owner", Object: "object"}, {Subject: "owner", Object: "object"}},
			want: []RefEdge{{Subject: "owner", Object: "object"}},
		},
		{
			name:    "absent removal",
			removes: []RefEdge{{Subject: "missing", Object: "object"}},
			reject:  []RefEdge{{Subject: NodeUnreferenced, Object: "object"}},
		},
		{
			name:    "final owner orphan mark",
			seed:    []RefEdge{{Subject: "owner", Object: "object"}},
			removes: []RefEdge{{Subject: "owner", Object: "object"}},
			want:    []RefEdge{{Subject: NodeUnreferenced, Object: "object"}},
			reject:  []RefEdge{{Subject: "owner", Object: "object"}},
		},
		{
			name:    "permanent root is not orphaned",
			seed:    []RefEdge{{Subject: "owner", Object: NodeGCRoot}},
			removes: []RefEdge{{Subject: "owner", Object: NodeGCRoot}},
			reject:  []RefEdge{{Subject: NodeUnreferenced, Object: NodeGCRoot}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Start a graph and apply the case's ownership transition.
			graph := NewRefGraph(store_kvtx_inmem.NewStore())
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
	graph := NewRefGraph(store_kvtx_inmem.NewStore())
	rejected := errors.New("reject transaction")
	edge := RefEdge{Subject: "owner", Object: "object"}

	// Roll back after staging both graph directions.
	err := kvtx.RunTransaction(
		ctx,
		true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return graph.store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := graph.applyRefBatchTx(ctx, tx, &counter{}, []RefEdge{edge}, nil); err != nil {
				return err
			}
			return rejected
		},
	)
	if !errors.Is(err, rejected) {
		t.Fatalf("error = %v, want %v", err, rejected)
	}

	// Confirm rollback removed the forward and reverse records.
	assertEdge(t, graph, edge, false)
}

// assertEdge checks that both stored directions agree on edge existence.
func assertEdge(t testing.TB, graph *RefGraph, edge RefEdge, want bool) {
	t.Helper()
	assertRecord(t, graph, graphKey(kindForward, edge.Subject, edge.Object), want)
	assertRecord(t, graph, graphKey(kindIncoming, edge.Object, edge.Subject), want)
}

// assertRecord checks one graph record through a fresh read transaction.
func assertRecord(t testing.TB, graph *RefGraph, key []byte, want bool) {
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
