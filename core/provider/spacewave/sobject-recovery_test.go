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
	var calls []string
	_, entityPID := generateTestKeypair(t)
	const soID = "so-space-create"

	var (
		acc         *ProviderAccount
		postedRoot  *sobject.SORoot
		postedEpoch *sobject.SOKeyEpoch
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
		case "/api/session/write-tickets/" + soID:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.WriteTicketBundleResponse{
				SoRootTicket: "ticket-root",
			}))
		case "/api/sobject/" + soID + "/root":
			if got := r.Header.Get("X-Write-Ticket"); got != "ticket-root" {
				t.Fatalf("unexpected write ticket: %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read root body: %v", err)
			}
			req := &api.PostRootRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal root request: %v", err)
			}
			postedRoot = req.GetRoot()
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc = NewTestProviderAccount(t, srv.URL)
	meta, err := space.NewSharedObjectMeta("Seeded Space")
	if err != nil {
		t.Fatalf("NewSharedObjectMeta: %v", err)
	}
	if _, err := acc.CreateSharedObject(context.Background(), soID, meta, "", ""); err != nil {
		t.Fatalf("CreateSharedObject: %v", err)
	}
	if postedRoot == nil {
		t.Fatal("expected root write")
	}
	if postedEpoch == nil {
		t.Fatal("expected key epoch in config-state")
	}
	inner := decodePostedRootInner(
		t,
		soID,
		acc.sessionClient.priv,
		acc.sessionClient.peerID.String(),
		postedEpoch,
		postedRoot,
	)
	worldState := &sobject_world_engine.InnerState{}
	if err := worldState.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatalf("unmarshal world state: %v", err)
	}
	if worldState.GetHeadRef().GetEmpty() {
		t.Fatal("expected initialized world head ref")
	}

	expectedCalls := []string{
		"POST /api/sobject/" + soID + "/create",
		"GET /api/sobject/" + soID + "/recovery-entity-keypairs",
		"POST /api/sobject/" + soID + "/config-state",
		"POST /api/session/write-tickets/" + soID,
		"POST /api/sobject/" + soID + "/root",
	}
	if !slices.Equal(calls, expectedCalls) {
		t.Fatalf("unexpected call sequence: %v", calls)
	}
}

func TestEnsureAccountSettingsSharedObject_CreatesWhenMissing(t *testing.T) {
	var calls []string
	_, entityPID := generateTestKeypair(t)
	const soID = "so-123"

	var (
		acc         *ProviderAccount
		postedRoot  *sobject.SORoot
		postedEpoch *sobject.SOKeyEpoch
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
		case "/api/session/write-tickets/" + soID:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.WriteTicketBundleResponse{
				SoRootTicket: "ticket-root",
			}))
		case "/api/sobject/" + soID + "/root":
			if got := r.Header.Get("X-Write-Ticket"); got != "ticket-root" {
				t.Fatalf("unexpected write ticket: %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read root body: %v", err)
			}
			req := &api.PostRootRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal root request: %v", err)
			}
			postedRoot = req.GetRoot()
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
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

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

	expectedCalls := []string{
		"POST /api/account/sobject-binding/ensure",
		"POST /api/sobject/" + soID + "/create",
		"GET /api/sobject/" + soID + "/recovery-entity-keypairs",
		"POST /api/sobject/" + soID + "/config-state",
		"POST /api/session/write-tickets/" + soID,
		"POST /api/sobject/" + soID + "/root",
		"POST /api/account/sobject-binding/finalize",
	}
	if !slices.Equal(calls, expectedCalls) {
		t.Fatalf("unexpected call sequence: %v", calls)
	}

	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 1 {
		t.Fatalf("expected account settings ensure to refresh SO list cache, got %#v", list)
	}
	if got := list.GetSharedObjects()[0].GetRef().GetProviderResourceRef().GetId(); got != soID {
		t.Fatalf("expected cached SO id %q, got %q", soID, got)
	}

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
	if postedRoot == nil {
		t.Fatal("expected root write")
	}
	if postedEpoch == nil {
		t.Fatal("expected key epoch in config-state")
	}
	inner := decodePostedRootInner(
		t,
		soID,
		acc.sessionClient.priv,
		acc.sessionClient.peerID.String(),
		postedEpoch,
		postedRoot,
	)
	if len(inner.GetStateData()) != 0 {
		t.Fatalf("expected account settings root state to be empty, got %d bytes", len(inner.GetStateData()))
	}
}
