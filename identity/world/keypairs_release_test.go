package identity_world

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/identity"
	identity_domain "github.com/s4wave/spacewave/identity/domain"
	"github.com/s4wave/spacewave/net/peer"
)

// countingWorldState counts ObjectState releases issued through GetObject.
type countingWorldState struct {
	world.WorldState
	released *int
}

func (c *countingWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	obj, found, err := c.WorldState.GetObject(ctx, key)
	if err != nil || !found || obj == nil {
		return obj, found, err
	}
	return &countingObjectState{ObjectState: obj, released: c.released}, true, nil
}

// countingObjectState delegates to the wrapped state and counts Release calls.
type countingObjectState struct {
	world.ObjectState
	released *int
}

func (o *countingObjectState) Release() {
	*o.released++
	world.ReleaseObjectState(o.ObjectState)
}

// TestIdentityBatchLookupsReleaseStates fails if the identity batch lookups
// leave their remote-releasable ObjectState handles alive.
func TestIdentityBatchLookupsReleaseStates(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	w := tb.WorldState

	// Store one keypair, entity, and domain info fixture each.
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	kp, err := identity.NewKeypair(p.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := StoreKeypair(ctx, w, p.GetPeerID(), kp, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := StoreEntity(ctx, w, p.GetPeerID(), identity.NewEntity(
		"test-domain",
		"test-entity",
		"0b6e1d11-1a5f-4c3e-9d2a-7f1e2b3c4d5e",
	)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := StoreDomainInfo(ctx, w, p.GetPeerID(), &identity_domain.DomainInfo{
		DomainId: "test-domain",
		Name:     "test",
	}); err != nil {
		t.Fatal(err)
	}

	var released int
	cws := &countingWorldState{WorldState: w, released: &released}

	kps, err := LookupKeypairs(ctx, cws, []string{NewKeypairKey(kp.GetPeerId()), NewKeypairKey("missing-peer")})
	if err != nil {
		t.Fatal(err)
	}
	if len(kps) != 2 || kps[0] == nil || kps[1] != nil {
		t.Fatalf("expected one found keypair and one missing, got %#v", kps)
	}
	if released != 1 {
		t.Fatalf("LookupKeypairs: released %d states, want 1", released)
	}

	ents, err := LookupEntities(ctx, cws, []string{NewEntityKey("test-domain", "test-entity")})
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0] == nil {
		t.Fatalf("expected one entity, got %#v", ents)
	}
	if released != 2 {
		t.Fatalf("LookupEntities: released %d states total, want 2", released)
	}

	dis, err := LookupDomainInfos(ctx, cws, []string{NewDomainInfoKey("test-domain")})
	if err != nil {
		t.Fatal(err)
	}
	if len(dis) != 1 || dis[0] == nil {
		t.Fatalf("expected one domain info, got %#v", dis)
	}
	if released != 3 {
		t.Fatalf("LookupDomainInfos: released %d states total, want 3", released)
	}
}
