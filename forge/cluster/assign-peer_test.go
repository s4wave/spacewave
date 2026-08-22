package forge_cluster_test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestAssignClusterLeaderPeerUpdatesCluster pins that assigning a new cluster
// leader rewrites the cluster peer and relinks the keypair.
func TestAssignClusterLeaderPeerUpdatesCluster(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()
	ws := world.NewEngineWorldState(wtb.Engine, true)

	sk1, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldLeader, err := peer.IDFromPrivateKey(sk1)
	if err != nil {
		t.Fatal(err)
	}
	sk2, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	newLeader, err := peer.IDFromPrivateKey(sk2)
	if err != nil {
		t.Fatal(err)
	}

	clusterKey := "cluster/test-cluster"
	if _, _, err := forge_cluster.CreateCluster(ctx, ws, clusterKey, "test-cluster", oldLeader, oldLeader); err != nil {
		t.Fatal(err)
	}

	if _, _, err := forge_cluster.AssignClusterLeaderPeer(ctx, ws, oldLeader, clusterKey, newLeader); err != nil {
		t.Fatal(err)
	}

	var gotPeer string
	_, _, err = world.AccessWorldObject(ctx, ws, clusterKey, false, func(bcs *block.Cursor) error {
		var berr error
		var clstr *forge_cluster.Cluster
		clstr, berr = forge_cluster.UnmarshalCluster(ctx, bcs)
		if berr != nil {
			return berr
		}
		gotPeer = clstr.GetPeerId()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPeer != newLeader.String() {
		t.Fatalf("expected cluster peer %s after assignment, got %q", newLeader.String(), gotPeer)
	}

	kpKeys, err := identity_world.ListObjectKeypairs(ctx, ws, clusterKey)
	if err != nil {
		t.Fatal(err)
	}
	foundNew := false
	for _, kpKey := range kpKeys {
		if kpKey == identity_world.NewKeypairKey(newLeader.String()) {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatalf("expected keypair for new leader linked, got %v", kpKeys)
	}
}
