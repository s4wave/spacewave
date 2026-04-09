package world_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
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
