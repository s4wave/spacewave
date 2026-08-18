package forge_world_test

import (
	"context"
	"testing"

	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_world "github.com/s4wave/spacewave/forge/world"
)

func TestLookupWorldOpReconstructsClusterJobTransitions(t *testing.T) {
	start, err := forge_world.LookupWorldOp(context.Background(), forge_cluster.ClusterStartJobOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := start.(*forge_cluster.ClusterStartJobOp); !ok {
		t.Fatalf("lookup %q returned %T", forge_cluster.ClusterStartJobOpId, start)
	}

	complete, err := forge_world.LookupWorldOp(context.Background(), forge_cluster.ClusterCompleteJobOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := complete.(*forge_cluster.ClusterCompleteJobOp); !ok {
		t.Fatalf("lookup %q returned %T", forge_cluster.ClusterCompleteJobOpId, complete)
	}
}
