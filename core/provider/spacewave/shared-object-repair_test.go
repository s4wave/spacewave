package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
)

func TestAuthorizeSharedObjectMutationAccountOwner(t *testing.T) {
	t.Parallel()

	acc := NewTestProviderAccount(t, "http://example.invalid")
	if err := acc.authorizeSharedObjectMutation(
		context.Background(),
		&api.SpaceMetadataResponse{
			OwnerType: sobject.OwnerTypeAccount,
			OwnerId:   acc.accountID,
		},
		"space-1",
	); err != nil {
		t.Fatalf("authorizeSharedObjectMutation() = %v", err)
	}
}

func TestAuthorizeSharedObjectMutationRejectsNonOwnerAccount(t *testing.T) {
	t.Parallel()

	acc := NewTestProviderAccount(t, "http://example.invalid")
	err := acc.authorizeSharedObjectMutation(
		context.Background(),
		&api.SpaceMetadataResponse{
			OwnerType: sobject.OwnerTypeAccount,
			OwnerId:   "someone-else",
		},
		"space-1",
	)
	if err == nil {
		t.Fatal("expected account-owner authorization error")
	}
}

func TestAuthorizeSharedObjectMutationRejectsNonOwnerOrgMember(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/org/list":
			body, err = (&api.ListOrgsResponse{
				Organizations: []*api.OrgResponse{{
					Id:          "org-1",
					DisplayName: "Org One",
					Role:        "org:member",
				}},
			}).MarshalVT()
		case "/api/org/org-1":
			body, err = (&api.GetOrgResponse{
				Id:          "org-1",
				DisplayName: "Org One",
			}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	err := acc.authorizeSharedObjectMutation(
		context.Background(),
		&api.SpaceMetadataResponse{
			OwnerType: sobject.OwnerTypeOrganization,
			OwnerId:   "org-1",
		},
		"space-1",
	)
	if err == nil {
		t.Fatal("expected org-owner authorization error")
	}
}

func TestAuthorizeSharedObjectMutationAllowsOrgOwner(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/org/list":
			body, err = (&api.ListOrgsResponse{
				Organizations: []*api.OrgResponse{{
					Id:          "org-1",
					DisplayName: "Org One",
					Role:        "org:owner",
				}},
			}).MarshalVT()
		case "/api/org/org-1":
			body, err = (&api.GetOrgResponse{
				Id:          "org-1",
				DisplayName: "Org One",
			}).MarshalVT()
		case "/api/org/org-1/invites":
			body, err = (&api.ListOrgInvitesResponse{}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if err := acc.authorizeSharedObjectMutation(
		context.Background(),
		&api.SpaceMetadataResponse{
			OwnerType: sobject.OwnerTypeOrganization,
			OwnerId:   "org-1",
		},
		"space-1",
	); err != nil {
		t.Fatalf("authorizeSharedObjectMutation() = %v", err)
	}
}

func TestReinitializeSharedObjectClearsVerifiedCacheBeforeReseed(t *testing.T) {
	const soID = "so-reinitialize-cache"

	var acc *ProviderAccount
	var postedConfig bool
	var postedRoot bool
	var postedEpoch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/meta":
			body, err := (&api.SpaceMetadataResponse{
				ObjectType: "space",
				OwnerType:  sobject.OwnerTypeAccount,
				OwnerId:    acc.accountID,
			}).MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(body)
		case "/api/sobject/" + soID + "/reinitialize":
			body, err := (&api.ReinitializeSObjectResponse{}).MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(body)
		case "/api/sobject/" + soID + "/state":
			state := &sobject.SOState{
				Config: &sobject.SharedObjectConfig{
					Participants: []*sobject.SOParticipantConfig{{
						PeerId:   acc.sessionClient.peerID.String(),
						Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: acc.accountID,
					}},
				},
			}
			_, _ = w.Write(mustMarshalSOStateMessageSnapshotJSON(t, state))
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(mustMarshalVT(t, &sobject.SOConfigChainResponse{}))
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(mustMarshalVT(t, &api.ListSORecoveryEntityKeypairsResponse{}))
		case "/api/sobject/" + soID + "/config-state":
			postedConfig = true
			postedEpoch = configStateIncludesKeyEpoch(t, r)
			w.WriteHeader(http.StatusOK)
		case "/api/session/write-tickets/" + soID:
			body, err := (&api.WriteTicketBundleResponse{
				SoRootTicket: "root-ticket",
			}).MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(body)
		case "/api/sobject/" + soID + "/root":
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read root body: %v", err)
			}
			postedRoot = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc = NewTestProviderAccount(t, srv.URL)
	acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc.sobjects = keyed.NewKeyedRefCount[string, *sobjectTracker](
		func(key string) (keyed.Routine, *sobjectTracker) {
			return nil, nil
		},
	)
	if err := acc.writeVerifiedSOStateCache(context.Background(), soID, &api.VerifiedSOStateCache{
		GenesisHash:             []byte("old-genesis"),
		VerifiedConfigChainHash: []byte("old-head"),
	}); err != nil {
		t.Fatalf("write verified cache: %v", err)
	}

	if err := acc.ReinitializeSharedObject(context.Background(), soID); err != nil {
		t.Fatalf("ReinitializeSharedObject: %v", err)
	}

	cache, err := acc.loadVerifiedSOStateCache(context.Background(), soID)
	if err != nil {
		t.Fatalf("load verified cache: %v", err)
	}
	if cache != nil {
		t.Fatalf("expected verified cache cleared, got %+v", cache)
	}
	if !postedConfig {
		t.Fatal("expected config-state reseed")
	}
	if !postedRoot {
		t.Fatal("expected root reseed")
	}
	if !postedEpoch {
		t.Fatal("expected key epoch reseed")
	}
}

func TestRepairStandaloneEmptyRootClearsVerifiedCacheBeforeReseed(t *testing.T) {
	const soID = "so-standalone-empty-root"

	var acc *ProviderAccount
	var postedConfig bool
	var postedRoot bool
	var postedEpoch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			writeEmptyOwnerState(t, w, acc)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(mustMarshalVT(t, &sobject.SOConfigChainResponse{}))
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(mustMarshalVT(t, &api.ListSORecoveryEntityKeypairsResponse{}))
		case "/api/sobject/" + soID + "/config-state":
			postedConfig = true
			postedEpoch = configStateIncludesKeyEpoch(t, r)
			w.WriteHeader(http.StatusOK)
		case "/api/session/write-tickets/" + soID:
			writeRootTicketBundle(t, w)
		case "/api/sobject/" + soID + "/root":
			drainTestRequestBody(t, r)
			postedRoot = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc = newSharedObjectRepairCacheTestAccount(t, srv.URL, soID)
	_, sessionPriv, _, err := acc.getReadySessionClient(context.Background())
	if err != nil {
		t.Fatalf("get ready session client: %v", err)
	}
	if err := acc.repairStandaloneSharedObject(
		context.Background(),
		acc.sessionClient,
		sessionPriv,
		soID,
	); err != nil {
		t.Fatalf("repairStandaloneSharedObject: %v", err)
	}

	assertVerifiedCacheCleared(t, acc, soID)
	if !postedConfig {
		t.Fatal("expected config-state reseed")
	}
	if !postedRoot {
		t.Fatal("expected root reseed")
	}
	if !postedEpoch {
		t.Fatal("expected key epoch reseed")
	}
}

func TestRepairOrganizationRootEmptyRootClearsVerifiedCacheBeforeReseed(t *testing.T) {
	const orgID = "org-empty-root"

	var acc *ProviderAccount
	var postedConfig bool
	var postedRoot bool
	var postedEpoch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + orgID + "/state":
			writeEmptyOwnerState(t, w, acc)
		case "/api/sobject/" + orgID + "/config-chain":
			_, _ = w.Write(mustMarshalVT(t, &sobject.SOConfigChainResponse{}))
		case "/api/sobject/" + orgID + "/recovery-entity-keypairs":
			_, _ = w.Write(mustMarshalVT(t, &api.ListSORecoveryEntityKeypairsResponse{}))
		case "/api/sobject/" + orgID + "/config-state":
			postedConfig = true
			postedEpoch = configStateIncludesKeyEpoch(t, r)
			w.WriteHeader(http.StatusOK)
		case "/api/session/write-tickets/" + orgID:
			writeRootTicketBundle(t, w)
		case "/api/sobject/" + orgID + "/root":
			drainTestRequestBody(t, r)
			postedRoot = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	acc = newSharedObjectRepairCacheTestAccount(t, srv.URL, orgID)
	_, sessionPriv, _, err := acc.getReadySessionClient(context.Background())
	if err != nil {
		t.Fatalf("get ready session client: %v", err)
	}
	err = acc.repairOrganizationRootSharedObject(
		context.Background(),
		acc.sessionClient,
		sessionPriv,
		orgID,
	)
	if err == nil {
		t.Fatal("expected organization info fetch to fail after reseed")
	}

	assertVerifiedCacheCleared(t, acc, orgID)
	if !postedConfig {
		t.Fatal("expected config-state reseed")
	}
	if !postedRoot {
		t.Fatal("expected root reseed")
	}
	if !postedEpoch {
		t.Fatal("expected key epoch reseed")
	}
}

func newSharedObjectRepairCacheTestAccount(
	t *testing.T,
	endpoint string,
	sharedObjectID string,
) *ProviderAccount {
	t.Helper()

	acc := NewTestProviderAccount(t, endpoint)
	acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc.sobjects = keyed.NewKeyedRefCount[string, *sobjectTracker](
		func(key string) (keyed.Routine, *sobjectTracker) {
			return nil, nil
		},
	)
	if err := acc.writeVerifiedSOStateCache(context.Background(), sharedObjectID, &api.VerifiedSOStateCache{
		GenesisHash:             []byte("old-genesis"),
		VerifiedConfigChainHash: []byte("old-head"),
	}); err != nil {
		t.Fatalf("write verified cache: %v", err)
	}
	return acc
}

func writeEmptyOwnerState(
	t *testing.T,
	w http.ResponseWriter,
	acc *ProviderAccount,
) {
	t.Helper()

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId:   acc.sessionClient.peerID.String(),
				Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
				EntityId: acc.accountID,
			}},
		},
	}
	_, _ = w.Write(mustMarshalSOStateMessageSnapshotJSON(t, state))
}

func writeRootTicketBundle(t *testing.T, w http.ResponseWriter) {
	t.Helper()

	body, err := (&api.WriteTicketBundleResponse{
		SoRootTicket: "root-ticket",
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(body)
}

func drainTestRequestBody(t *testing.T, r *http.Request) {
	t.Helper()

	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		t.Fatalf("read request body: %v", err)
	}
}

func configStateIncludesKeyEpoch(t *testing.T, r *http.Request) bool {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read config-state body: %v", err)
	}
	req := &api.PostConfigStateRequest{}
	if err := req.UnmarshalVT(body); err != nil {
		t.Fatalf("unmarshal config-state request: %v", err)
	}
	return req.GetKeyEpoch() != nil
}

func assertVerifiedCacheCleared(
	t *testing.T,
	acc *ProviderAccount,
	sharedObjectID string,
) {
	t.Helper()

	cache, err := acc.loadVerifiedSOStateCache(context.Background(), sharedObjectID)
	if err != nil {
		t.Fatalf("load verified cache: %v", err)
	}
	if cache != nil {
		t.Fatalf("expected verified cache cleared, got %+v", cache)
	}
}
