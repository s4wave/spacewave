package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestRecoveryEntityKeypairsFollowReadableEntities checks that envelope builds
// list recovery keypairs again only when the config's readable entities change,
// reuse a listing across the account's session clients, and never reuse a
// listing that lacks an entity's keypairs.
func TestRecoveryEntityKeypairsFollowReadableEntities(t *testing.T) {
	// Give alice and bob one keypair each, and carol none.
	const soID = "so-keypair-cache"
	_, alicePID := generateTestKeypair(t)
	_, bobPID := generateTestKeypair(t)
	keypairs := map[string][]*session.EntityKeypair{
		"alice": {{PeerId: alicePID.String()}},
		"bob":   {{PeerId: bobPID.String()}},
	}

	// Start a Cloud that lists the keypairs of its config's readable entities
	// and counts its listings.
	var cloudEntities atomic.Pointer[[]string]
	var listings atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve only the keypair listing.
		if r.URL.Path != "/api/sobject/"+soID+"/recovery-entity-keypairs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Count it and list the Cloud config's entities.
		listings.Add(1)
		resp := &api.ListSORecoveryEntityKeypairsResponse{}
		for _, entityID := range *cloudEntities.Load() {
			resp.Entities = append(resp.Entities, &api.SORecoveryEntityKeypairs{
				EntityId: entityID,
				Keypairs: keypairs[entityID],
			})
		}
		_, _ = w.Write(mustMarshalVT(t, resp))
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	cli := acc.sessionClient

	// build seals envelopes for a config of entity peers, which the Cloud
	// also holds, and checks the listing count.
	build := func(wantListings int32, entityPeers ...string) error {
		// Build the config and the Cloud's entity list.
		t.Helper()
		cfg := &sobject.SharedObjectConfig{}
		var entities []string
		for i := 0; i < len(entityPeers); i += 2 {
			cfg.Participants = append(cfg.Participants, &sobject.SOParticipantConfig{
				PeerId:   entityPeers[i+1],
				EntityId: entityPeers[i],
				Role:     sobject.SOParticipantRole_SOParticipantRole_WRITER,
			})
			if len(entities) == 0 || entities[len(entities)-1] != entityPeers[i] {
				entities = append(entities, entityPeers[i])
			}
		}

		// Seal the envelopes and check the count.
		cloudEntities.Store(&entities)
		_, err := buildSORecoveryEnvelopes(context.Background(), cli, soID, cfg, 1, &sobject.SOGrantInner{})
		if got := listings.Load(); got != wantListings {
			t.Fatalf("listings = %d, want %d", got, wantListings)
		}
		return err
	}

	// A new peer of a listed entity reuses the listing, also through a
	// replacement session client.
	if err := build(1, "alice", "peer-a1"); err != nil {
		t.Fatal(err)
	}
	priv, pid := generateTestKeypair(t)
	acc.ReplaceSessionClient(NewSessionClient(http.DefaultClient, srv.URL, "", priv, pid.String()))
	cli = acc.sessionClient
	if err := build(1, "alice", "peer-a1", "alice", "peer-a2"); err != nil {
		t.Fatal(err)
	}

	// Adding or removing a readable entity lists again.
	if err := build(2, "alice", "peer-a1", "bob", "peer-b1"); err != nil {
		t.Fatal(err)
	}
	if err := build(2, "alice", "peer-a1", "bob", "peer-b1", "bob", "peer-b2"); err != nil {
		t.Fatal(err)
	}
	if err := build(3, "alice", "peer-a1"); err != nil {
		t.Fatal(err)
	}

	// A listing that lacks an entity's keypairs is never reused.
	var missing *missingRecoveryKeypairsError
	if err := build(4, "alice", "peer-a1", "carol", "peer-c1"); !errors.As(err, &missing) {
		t.Fatalf("expected missing keypairs for carol, got %v", err)
	}
	if err := build(5, "alice", "peer-a1", "carol", "peer-c1"); !errors.As(err, &missing) {
		t.Fatalf("expected missing keypairs for carol, got %v", err)
	}
}
