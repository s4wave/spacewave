package world_block

import (
	"context"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestBuildGCTreeInitFlag tracks whether the persisted GC graph still needs the
// permanent gcroot -> world edge initialized.
func TestBuildGCTreeInitFlag(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	ws, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	gcTree, refGraph, initGCRootEdge, err := ws.buildGCTree(ctx, ws.bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if initGCRootEdge {
		t.Fatal("expected initialized WorldState to skip gcroot reinitialization")
	}
	_ = refGraph.Close()
	gcTree.Discard()

	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	reopened, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	gcTree, refGraph, initGCRootEdge, err = reopened.buildGCTree(ctx, reopened.bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if initGCRootEdge {
		t.Fatal("expected persisted GC graph to skip gcroot reinitialization")
	}
	_ = refGraph.Close()
	gcTree.Discard()
}

// TestSetBlockTransactionCarriesRefGraphIRIRefKeys keeps the positive IRI
// ref-key cache across writable refgraph rebuilds.
func TestSetBlockTransactionCarriesRefGraphIRIRefKeys(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	ws, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ws.refGraph == nil {
		t.Fatal("expected writable world state to build a refgraph")
	}

	cached := map[string]any{
		block_gc.NodeUnreferenced: "cached-unreferenced",
		block_gc.ObjectIRI("foo"): "cached-object",
	}
	ws.refGraph.ImportIRIRefKeys(cached)

	if err := ws.SetBlockTransaction(ctx, ws.btx, ws.bcs); err != nil {
		t.Fatal(err.Error())
	}
	got := ws.refGraph.CloneIRIRefKeys()
	for iri, key := range cached {
		if got[iri] != key {
			t.Fatalf("expected cached key for %q to survive rebuild", iri)
		}
	}
}
