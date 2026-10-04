package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestProviderAccountCreateSpaceSeedsWorldHead(t *testing.T) {
	// Serve the Cloud routes a Space create uses, recording each call.
	var calls []string
	_, entityPID := generateTestKeypair(t)
	const soID = "so-space-create"
	var (
		acc              *ProviderAccount
		postedCheckpoint *sobject.SOCheckpoint
		postedEpoch      *sobject.SOKeyEpoch
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/create":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			req := &api.CreateSObjectRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal create request: %v", err)
			}
			if req.GetObjectType() != space.SpaceBodyType {
				t.Fatalf("unexpected object type: %q", req.GetObjectType())
			}
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.ListSORecoveryEntityKeypairsResponse{
				Entities: []*api.SORecoveryEntityKeypairs{{
					EntityId: "test-account",
					Keypairs: []*session.EntityKeypair{{
						PeerId: entityPID.String(),
					}},
				}},
			}))
		case "/api/sobject/" + soID + "/config-state":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read config-state body: %v", err)
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal config-state request: %v", err)
			}
			postedEpoch = req.GetKeyEpoch()
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/checkpoint":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read checkpoint body: %v", err)
			}
			req := &api.PostCheckpointRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal checkpoint request: %v", err)
			}
			postedCheckpoint = req.GetCheckpoint()
			w.WriteHeader(http.StatusOK)
		case "/api/account/state":
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{EntityId: "alice"}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	// Create a Space through the provider account.
	acc = NewTestProviderAccount(t, srv.URL)
	meta, err := space.NewSharedObjectMeta("Seeded Space")
	if err != nil {
		t.Fatalf("NewSharedObjectMeta: %v", err)
	}
	if _, err := acc.CreateSharedObject(context.Background(), soID, meta, "", ""); err != nil {
		t.Fatalf("CreateSharedObject: %v", err)
	}

	// Check that the posted checkpoint seeds a World head.
	if postedCheckpoint == nil {
		t.Fatal("expected checkpoint write")
	}
	if postedEpoch == nil {
		t.Fatal("expected key epoch in config-state")
	}
	stateData := decodePostedCheckpointState(
		t,
		soID,
		acc.sessionClient.priv,
		acc.sessionClient.peerID.String(),
		postedEpoch,
		postedCheckpoint,
	)
	worldState := &sobject_world_engine.InnerState{}
	if err := worldState.UnmarshalVT(stateData); err != nil {
		t.Fatalf("unmarshal world state: %v", err)
	}
	if worldState.GetHeadRef().GetEmpty() {
		t.Fatal("expected initialized world head ref")
	}

	// Check the Cloud call order.
	expectedCalls := []string{
		"POST /api/sobject/" + soID + "/create",
		"GET /api/account/state",
		"GET /api/sobject/" + soID + "/recovery-entity-keypairs",
		"POST /api/sobject/" + soID + "/config-state",
		"POST /api/sobject/" + soID + "/checkpoint",
	}
	if !slices.Equal(calls, expectedCalls) {
		t.Fatalf("unexpected call sequence: %v", calls)
	}
}

func TestEnsureAccountSettingsSharedObject_CreatesWhenMissing(t *testing.T) {
	// Serve the Cloud routes a settings object create uses, recording each call.
	var calls []string
	_, entityPID := generateTestKeypair(t)
	const soID = "so-123"
	var (
		acc              *ProviderAccount
		postedCheckpoint *sobject.SOCheckpoint
		postedEpoch      *sobject.SOKeyEpoch
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/account/sobject-binding/ensure":
			_, _ = w.Write(mustMarshalVT(t, &api.EnsureAccountSObjectBindingResponse{
				Binding: &api.AccountSObjectBinding{
					Purpose: "account-settings",
					SoId:    soID,
					State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED,
				},
			}))
		case "/api/sobject/" + soID + "/create":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			req := &api.CreateSObjectRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal create request: %v", err)
			}
			if req.GetObjectType() != "account-settings" {
				t.Fatalf("unexpected object type: %q", req.GetObjectType())
			}
			if req.GetOwnerType() != sobject.OwnerTypeAccount {
				t.Fatalf("unexpected owner type: %q", req.GetOwnerType())
			}
			if req.GetOwnerId() != "test-account" {
				t.Fatalf("unexpected owner id: %q", req.GetOwnerId())
			}
			if !req.GetAccountPrivate() {
				t.Fatalf("expected account-private create request")
			}
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/config-state":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read config-state body: %v", err)
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal config-state request: %v", err)
			}
			postedEpoch = req.GetKeyEpoch()
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/checkpoint":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read checkpoint body: %v", err)
			}
			req := &api.PostCheckpointRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal checkpoint request: %v", err)
			}
			postedCheckpoint = req.GetCheckpoint()
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.ListSORecoveryEntityKeypairsResponse{
				Entities: []*api.SORecoveryEntityKeypairs{{
					EntityId: "test-account",
					Keypairs: []*session.EntityKeypair{{
						PeerId: entityPID.String(),
					}},
				}},
			}))
		case "/api/account/sobject-binding/finalize":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.FinalizeAccountSObjectBindingResponse{
				Binding: &api.AccountSObjectBinding{
					Purpose: "account-settings",
					SoId:    soID,
					State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
				},
			}))
		case "/api/account/state":
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{EntityId: "alice"}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	// Ensure the settings object on an active account.
	acc = NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE)
	ref, err := acc.ensureAccountSettingsSharedObject(context.Background())
	if err != nil {
		t.Fatalf("ensureAccountSettingsSharedObject: %v", err)
	}
	if ref.GetProviderResourceRef().GetId() != soID {
		t.Fatalf("unexpected shared object ID: %q", ref.GetProviderResourceRef().GetId())
	}
	if ref.GetBlockStoreId() != soID {
		t.Fatalf("unexpected block store ID: %q", ref.GetBlockStoreId())
	}

	// Check the Cloud call order.
	expectedCalls := []string{
		"POST /api/account/sobject-binding/ensure",
		"POST /api/sobject/" + soID + "/create",
		"GET /api/account/state",
		"GET /api/sobject/" + soID + "/recovery-entity-keypairs",
		"POST /api/sobject/" + soID + "/config-state",
		"POST /api/sobject/" + soID + "/checkpoint",
		"POST /api/account/sobject-binding/finalize",
	}
	if !slices.Equal(calls, expectedCalls) {
		t.Fatalf("unexpected call sequence: %v", calls)
	}

	// Check that the list cache holds the new object.
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 1 {
		t.Fatalf("expected account settings ensure to refresh SO list cache, got %#v", list)
	}
	if got := list.GetSharedObjects()[0].GetRef().GetProviderResourceRef().GetId(); got != soID {
		t.Fatalf("expected cached SO id %q, got %q", soID, got)
	}

	// Check the cached metadata.
	metadata, err := acc.GetSharedObjectMetadata(context.Background(), soID)
	if err != nil {
		t.Fatalf("get seeded shared object metadata: %v", err)
	}
	if metadata.GetOwnerType() != sobject.OwnerTypeAccount {
		t.Fatalf("unexpected cached owner type: %q", metadata.GetOwnerType())
	}
	if metadata.GetOwnerId() != "test-account" {
		t.Fatalf("unexpected cached owner id: %q", metadata.GetOwnerId())
	}
	if metadata.GetObjectType() != "account-settings" {
		t.Fatalf("unexpected cached object type: %q", metadata.GetObjectType())
	}

	// Check that the posted checkpoint holds empty settings state.
	if postedCheckpoint == nil {
		t.Fatal("expected checkpoint write")
	}
	if postedEpoch == nil {
		t.Fatal("expected key epoch in config-state")
	}
	stateData := decodePostedCheckpointState(
		t,
		soID,
		acc.sessionClient.priv,
		acc.sessionClient.peerID.String(),
		postedEpoch,
		postedCheckpoint,
	)
	if len(stateData) != 0 {
		t.Fatalf("expected account settings checkpoint state to be empty, got %d bytes", len(stateData))
	}
}
