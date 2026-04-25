package provider_spacewave

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// TestListSharedObjects_Success verifies ListSharedObjects sends GET to /sobject/list.
func TestListSharedObjects_Success(t *testing.T) {
	respBody := `{"sharedObjects":[{"id":"so-1"},{"id":"so-2"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/sobject/list") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Peer-ID") == "" {
			t.Error("missing X-Peer-ID")
		}
		if r.Header.Get("X-Signature") == "" {
			t.Error("missing X-Signature")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	data, err := cli.ListSharedObjects(context.Background())
	if err != nil {
		t.Fatalf("ListSharedObjects: %v", err)
	}
	if string(data) != respBody {
		t.Fatalf("unexpected response: %q", data)
	}
}

// TestListSharedObjects_ServerError verifies ListSharedObjects returns error on failure.
func TestListSharedObjects_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	_, err := cli.ListSharedObjects(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 status")
	}
}

// TestPostOp_MissingWriteTicketExecutor verifies PostOp fails locally when the
// write-ticket executor is unavailable.
func TestPostOp_MissingWriteTicketExecutor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	err := cli.PostOp(context.Background(), "my-so-id", []byte("op-data"))
	if err == nil || !strings.Contains(err.Error(), "missing write-ticket executor") {
		t.Fatalf("expected missing write-ticket executor error, got %v", err)
	}
}

// TestPostOp_UsesWriteTicketWhenConfigured verifies PostOp switches to the
// write-ticket proof path when the shared ticket executor is configured.
func TestPostOp_UsesWriteTicketWhenConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/sobject/my-so-id/op" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Write-Ticket"); got != "ticket-1" {
			t.Errorf("unexpected write ticket: %q", got)
		}
		if r.Header.Get("X-Signature") != "" {
			t.Error("signed auth should not be used on the write-ticket path")
		}
		if r.Header.Get("X-Peer-ID") != "" {
			t.Error("write-ticket path should not set X-Peer-ID on the outer request")
		}
		if got := r.Header.Get(SeedReasonHeader); got != string(SeedReasonMutation) {
			t.Errorf("unexpected seed reason: %q", got)
		}

		body, _ := io.ReadAll(r.Body)
		if string(body) != "op-data" {
			t.Errorf("unexpected body: %q", body)
		}

		proofB64 := r.Header.Get("X-Write-Proof")
		if proofB64 == "" {
			t.Fatal("missing X-Write-Proof")
		}
		proofBytes, err := base64.StdEncoding.DecodeString(proofB64)
		if err != nil {
			t.Fatalf("decode proof: %v", err)
		}
		var proof api.WriteTicketProof
		if err := proof.UnmarshalVT(proofBytes); err != nil {
			t.Fatalf("unmarshal proof: %v", err)
		}
		var payload api.WriteTicketProofPayload
		if err := payload.UnmarshalVT(proof.GetPayload()); err != nil {
			t.Fatalf("unmarshal proof payload: %v", err)
		}
		if payload.GetTicket() != "ticket-1" {
			t.Errorf("unexpected proof ticket: %q", payload.GetTicket())
		}
		if payload.GetMethod() != http.MethodPost {
			t.Errorf("unexpected proof method: %q", payload.GetMethod())
		}
		if payload.GetPath() != "/api/sobject/my-so-id/op" {
			t.Errorf("unexpected proof path: %q", payload.GetPath())
		}
		if payload.GetContentLength() != int64(len(body)) {
			t.Errorf("unexpected proof content length: %d", payload.GetContentLength())
		}
		wantHash := sha256.Sum256(body)
		if payload.GetBodyHashHex() != hex.EncodeToString(wantHash[:]) {
			t.Errorf("unexpected proof body hash: %q", payload.GetBodyHashHex())
		}
		if payload.GetSignedHeaders() != "content-type=application/octet-stream" {
			t.Errorf("unexpected proof signed headers: %q", payload.GetSignedHeaders())
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "my-so-id" {
			t.Errorf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceSOOp {
			t.Errorf("unexpected audience: %s", audience)
		}
		return fn("ticket-1")
	}

	if err := cli.PostOp(context.Background(), "my-so-id", []byte("op-data")); err != nil {
		t.Fatalf("PostOp: %v", err)
	}
}

// TestPostRoot_UsesWriteTicketWhenConfigured verifies PostRoot switches to the
// write-ticket proof path when the shared ticket executor is configured.
func TestPostRoot_UsesWriteTicketWhenConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/sobject/root-so-id/root" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Write-Ticket"); got != "ticket-root" {
			t.Errorf("unexpected write ticket: %q", got)
		}
		if r.Header.Get("X-Signature") != "" {
			t.Error("signed auth should not be used on the write-ticket path")
		}

		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Fatal("missing body")
		}

		proofB64 := r.Header.Get("X-Write-Proof")
		if proofB64 == "" {
			t.Fatal("missing X-Write-Proof")
		}
		proofBytes, err := base64.StdEncoding.DecodeString(proofB64)
		if err != nil {
			t.Fatalf("decode proof: %v", err)
		}
		var proof api.WriteTicketProof
		if err := proof.UnmarshalVT(proofBytes); err != nil {
			t.Fatalf("unmarshal proof: %v", err)
		}
		var payload api.WriteTicketProofPayload
		if err := payload.UnmarshalVT(proof.GetPayload()); err != nil {
			t.Fatalf("unmarshal proof payload: %v", err)
		}
		if payload.GetTicket() != "ticket-root" {
			t.Errorf("unexpected proof ticket: %q", payload.GetTicket())
		}
		if payload.GetMethod() != http.MethodPost {
			t.Errorf("unexpected proof method: %q", payload.GetMethod())
		}
		if payload.GetPath() != "/api/sobject/root-so-id/root" {
			t.Errorf("unexpected proof path: %q", payload.GetPath())
		}
		if payload.GetContentLength() != int64(len(body)) {
			t.Errorf("unexpected proof content length: %d", payload.GetContentLength())
		}
		wantHash := sha256.Sum256(body)
		if payload.GetBodyHashHex() != hex.EncodeToString(wantHash[:]) {
			t.Errorf("unexpected proof body hash: %q", payload.GetBodyHashHex())
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "root-so-id" {
			t.Errorf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceSORoot {
			t.Errorf("unexpected audience: %s", audience)
		}
		return fn("ticket-root")
	}

	err := cli.PostRoot(context.Background(), "root-so-id", &sobject.SORoot{
		InnerSeqno: 7,
		Inner:      []byte("root-bytes"),
	}, nil)
	if err != nil {
		t.Fatalf("PostRoot: %v", err)
	}
}

// TestPostOp_ServerError verifies PostOp returns error on server failure.
func TestPostOp_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-err")
	}

	err := cli.PostOp(context.Background(), "so-id", []byte("data"))
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

// TestCreateSharedObject_Success verifies CreateSharedObject sends the current
// binary request contract to the correct path.
func TestCreateSharedObject_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/sobject/new-so-id/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		req := &api.CreateSObjectRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal create request: %v", err)
		}
		if req.GetDisplayName() != "My Space" {
			t.Errorf("unexpected display name: %q", req.GetDisplayName())
		}
		if req.GetObjectType() != "space" {
			t.Errorf("unexpected object type: %q", req.GetObjectType())
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	err := cli.CreateSharedObject(
		context.Background(),
		"new-so-id",
		"My Space",
		"space",
		"",
		"",
		false,
	)
	if err != nil {
		t.Fatalf("CreateSharedObject: %v", err)
	}
}

// TestCreateSharedObject_ServerError verifies CreateSharedObject returns error on failure.
func TestCreateSharedObject_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("conflict"))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	err := cli.CreateSharedObject(context.Background(), "so-id", "", "space", "", "", false)
	if err == nil {
		t.Fatal("expected error for 409 status")
	}
}

func TestGetSharedObjectDisplayName(t *testing.T) {
	soMeta, err := space.NewSharedObjectMeta("My Space")
	if err != nil {
		t.Fatalf("NewSharedObjectMeta: %v", err)
	}

	if got := getSharedObjectDisplayName(soMeta); got != "My Space" {
		t.Fatalf("expected display name %q, got %q", "My Space", got)
	}

	if got := getSharedObjectDisplayName(&sobject.SharedObjectMeta{
		BodyType: "counter",
	}); got != "" {
		t.Fatalf("expected empty display name for non-space meta, got %q", got)
	}
}

func TestEnsureAccountSettingsSharedObject_CreatesWhenMissing(t *testing.T) {
	var calls []string
	_, entityPID := generateTestKeypair(t)
	const soID = "so-123"

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
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/key-epoch":
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

	acc := NewTestProviderAccount(t, srv.URL)
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
		"POST /api/sobject/" + soID + "/key-epoch",
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
}

func TestEnsureAccountSettingsSharedObject_AlreadyExists(t *testing.T) {
	var calls []string
	const soID = "so-123"

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
			w.WriteHeader(http.StatusConflict)
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

	acc := NewTestProviderAccount(t, srv.URL)

	ref, err := acc.ensureAccountSettingsSharedObject(context.Background())
	if err != nil {
		t.Fatalf("ensureAccountSettingsSharedObject: %v", err)
	}
	if ref.GetProviderResourceRef().GetId() != soID {
		t.Fatalf("unexpected shared object ID: %q", ref.GetProviderResourceRef().GetId())
	}

	expectedCalls := []string{
		"POST /api/account/sobject-binding/ensure",
		"POST /api/sobject/" + soID + "/create",
		"POST /api/account/sobject-binding/finalize",
	}
	if !slices.Equal(calls, expectedCalls) {
		t.Fatalf("unexpected call sequence: %v", calls)
	}
}

func TestEnsureAccountSettingsSharedObject_UsesReadyBindingFromAccountState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		AccountSobjectBindings: []*api.AccountSObjectBinding{{
			Purpose: "account-settings",
			SoId:    "so-123",
			State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
		}},
	}

	ref, err := acc.ensureAccountSettingsSharedObject(context.Background())
	if err != nil {
		t.Fatalf("ensureAccountSettingsSharedObject: %v", err)
	}
	if ref.GetProviderResourceRef().GetId() != "so-123" {
		t.Fatalf("unexpected shared object ID: %q", ref.GetProviderResourceRef().GetId())
	}
}

func TestDeleteSharedObjectRemovesMetadataAndListCaches(t *testing.T) {
	var calls []string
	const soID = "so-123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/delete":
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/list":
			t.Fatal("delete should not refresh the shared object list")
		case "/api/sobject/" + soID + "/meta":
			t.Fatal("delete should not fetch deleted metadata")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	meta, err := space.NewSharedObjectMeta("Deleted Space")
	if err != nil {
		t.Fatalf("build shared object metadata: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef(soID),
		Meta:   meta,
		Source: "cloud",
	})
	acc.SetSharedObjectMetadata(soID, &api.SpaceMetadataResponse{
		OwnerType:   sobject.OwnerTypeAccount,
		OwnerId:     "test-account",
		DisplayName: "Deleted Space",
		ObjectType:  "space",
	})

	if err := acc.DeleteSharedObject(context.Background(), soID); err != nil {
		t.Fatalf("delete shared object: %v", err)
	}
	if !slices.Equal(calls, []string{"DELETE /api/sobject/" + soID + "/delete"}) {
		t.Fatalf("unexpected calls: %v", calls)
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 0 {
		t.Fatalf("expected deleted shared object removed from list cache, got %#v", list)
	}
	if _, err := acc.GetSharedObjectMetadata(context.Background(), soID); err != ErrSharedObjectMetadataDeleted {
		t.Fatalf("expected deleted metadata tombstone, got %v", err)
	}
}

func mustMarshalVT(t *testing.T, marshaler interface{ MarshalVT() ([]byte, error) }) []byte {
	t.Helper()
	data, err := marshaler.MarshalVT()
	if err != nil {
		t.Fatalf("marshal protobuf: %v", err)
	}
	return data
}

func mustMarshalSOStateMessageSnapshotJSON(t *testing.T, state *sobject.SOState) []byte {
	t.Helper()
	return mustMarshalVT(t, &api.SOStateMessage{
		Seqno:   1,
		Content: &api.SOStateMessage_Snapshot{Snapshot: state},
	})
}

func TestEnsureSharedObjectListLoaded_NoSubscriptionSkipsFetch(t *testing.T) {
	var listCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sobject/list" {
			listCalls++
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	ctr, rel, err := acc.AccessSharedObjectList(context.Background(), nil)
	if err != nil {
		t.Fatalf("AccessSharedObjectList: %v", err)
	}
	defer rel()

	list, err := ctr.WaitValue(context.Background(), nil)
	if err != nil {
		t.Fatalf("WaitValue: %v", err)
	}
	if list == nil {
		t.Fatal("expected empty list value")
	}
	if len(list.GetSharedObjects()) != 0 {
		t.Fatalf("expected empty list, got %#v", list)
	}
	if listCalls != 0 {
		t.Fatalf("expected no list fetches, got %d", listCalls)
	}
}

func TestEnsureSharedObjectListLoaded_InvalidationRefetchesOnce(t *testing.T) {
	var listCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		listCalls++
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{}))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(
		s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	)

	if err := acc.EnsureSharedObjectListLoaded(context.Background()); err != nil {
		t.Fatalf("first EnsureSharedObjectListLoaded: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected 1 list fetch after first ensure, got %d", listCalls)
	}

	if err := acc.EnsureSharedObjectListLoaded(context.Background()); err != nil {
		t.Fatalf("second EnsureSharedObjectListLoaded: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected cached ensure to avoid refetch, got %d calls", listCalls)
	}

	acc.invalidateSharedObjectList()
	if err := acc.EnsureSharedObjectListLoaded(context.Background()); err != nil {
		t.Fatalf("third EnsureSharedObjectListLoaded: %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("expected invalidation to trigger one refetch, got %d calls", listCalls)
	}
}

// TestPostRoot_MissingWriteTicketExecutor verifies PostRoot fails locally when
// the write-ticket executor is unavailable.
func TestPostRoot_MissingWriteTicketExecutor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	err := cli.PostRoot(context.Background(), "root-so-id", &sobject.SORoot{
		Inner: []byte("root-data"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing write-ticket executor") {
		t.Fatalf("expected missing write-ticket executor error, got %v", err)
	}
}

// TestPostInitState_MissingWriteTicketExecutor verifies PostInitState fails
// locally when the write-ticket executor is unavailable.
func TestPostInitState_MissingWriteTicketExecutor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	rootData, err := (&sobject.SORoot{
		Inner: []byte("init-root-data"),
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root: %v", err)
	}

	err = cli.PostInitState(context.Background(), "init-so-id", rootData)
	if err == nil || !strings.Contains(err.Error(), "missing write-ticket executor") {
		t.Fatalf("expected missing write-ticket executor error, got %v", err)
	}
}

// TestPostInitState_UsesWriteTicketWhenConfigured verifies PostInitState uses
// the write-ticket proof path when the shared ticket executor is configured.
func TestPostInitState_UsesWriteTicketWhenConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/sobject/init-so-id/root" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Write-Ticket"); got != "ticket-init-root" {
			t.Errorf("unexpected write ticket: %q", got)
		}
		if r.Header.Get("X-Signature") != "" {
			t.Error("signed auth should not be used on the write-ticket path")
		}
		if r.Header.Get("X-Peer-ID") != "" {
			t.Error("write-ticket path should not set X-Peer-ID on the outer request")
		}
		if got := r.Header.Get(SeedReasonHeader); got != string(SeedReasonMutation) {
			t.Errorf("unexpected seed reason: %q", got)
		}

		body, _ := io.ReadAll(r.Body)
		req := &api.PostRootRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal post root request: %v", err)
		}
		if req.GetRoot() == nil {
			t.Fatal("expected root in post init request")
		}
		if string(req.GetRoot().GetInner()) != "init-root-data" {
			t.Errorf("unexpected root inner: %q", req.GetRoot().GetInner())
		}

		proofB64 := r.Header.Get("X-Write-Proof")
		if proofB64 == "" {
			t.Fatal("missing X-Write-Proof")
		}
		proofBytes, err := base64.StdEncoding.DecodeString(proofB64)
		if err != nil {
			t.Fatalf("decode proof: %v", err)
		}
		var proof api.WriteTicketProof
		if err := proof.UnmarshalVT(proofBytes); err != nil {
			t.Fatalf("unmarshal proof: %v", err)
		}
		var payload api.WriteTicketProofPayload
		if err := payload.UnmarshalVT(proof.GetPayload()); err != nil {
			t.Fatalf("unmarshal proof payload: %v", err)
		}
		if payload.GetTicket() != "ticket-init-root" {
			t.Errorf("unexpected proof ticket: %q", payload.GetTicket())
		}
		if payload.GetMethod() != http.MethodPost {
			t.Errorf("unexpected proof method: %q", payload.GetMethod())
		}
		if payload.GetPath() != "/api/sobject/init-so-id/root" {
			t.Errorf("unexpected proof path: %q", payload.GetPath())
		}
		if payload.GetContentLength() != int64(len(body)) {
			t.Errorf("unexpected proof content length: %d", payload.GetContentLength())
		}
		wantHash := sha256.Sum256(body)
		if payload.GetBodyHashHex() != hex.EncodeToString(wantHash[:]) {
			t.Errorf("unexpected proof body hash: %q", payload.GetBodyHashHex())
		}
		if payload.GetSignedHeaders() != "content-type=application/octet-stream" {
			t.Errorf("unexpected proof signed headers: %q", payload.GetSignedHeaders())
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "init-so-id" {
			t.Errorf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceSORoot {
			t.Errorf("unexpected audience: %s", audience)
		}
		return fn("ticket-init-root")
	}

	rootData, err := (&sobject.SORoot{
		Inner: []byte("init-root-data"),
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root: %v", err)
	}

	err = cli.PostInitState(context.Background(), "init-so-id", rootData)
	if err != nil {
		t.Fatalf("PostInitState: %v", err)
	}
}

// TestGetSOState_Success verifies GetSOState sends GET to the correct path.
func TestGetSOState_Success(t *testing.T) {
	respBody := `{"root":{}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/sobject/state-so-id/state" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	data, err := cli.GetSOState(context.Background(), "state-so-id", 0, SeedReasonColdSeed)
	if err != nil {
		t.Fatalf("GetSOState: %v", err)
	}
	if string(data) != respBody {
		t.Fatalf("unexpected response: %q", data)
	}
}

// TestGetSOState_ServerError verifies GetSOState returns error on failure.
func TestGetSOState_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	_, err := cli.GetSOState(context.Background(), "missing-id", 0, SeedReasonColdSeed)
	if err == nil {
		t.Fatal("expected error for 404 status")
	}
}

// TestSobjectBlockStoreID verifies the block store ID format.
func TestSobjectBlockStoreID(t *testing.T) {
	result := SobjectBlockStoreID("my-object")
	if result != "my-object" {
		t.Fatalf("unexpected block store ID: %q", result)
	}
}
