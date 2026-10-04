package provider_spacewave

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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

// TestSObjectWritesAreSigned checks that operations and checkpoints are
// signed session POSTs carrying their encoded bodies.
func TestSObjectWritesAreSigned(t *testing.T) {
	// Each write posts its encoded body to its action path.
	ops := []*sobject.SOOperation{{Inner: []byte("op")}}
	checkpoint := &sobject.SOCheckpoint{Inner: []byte("checkpoint")}
	opsBody, _ := (&api.PostOpsRequest{Operations: ops}).MarshalVT()
	checkpointBody, _ := (&api.PostCheckpointRequest{Checkpoint: checkpoint}).MarshalVT()
	cases := []struct {
		action string
		body   []byte
		post   func(cli *SessionClient) error
	}{
		{"op", []byte("op-data"), func(cli *SessionClient) error {
			return cli.PostOp(t.Context(), "so-1", []byte("op-data"))
		}},
		{"ops", opsBody, func(cli *SessionClient) error {
			return cli.PostOps(t.Context(), "so-1", ops)
		}},
		{"checkpoint", checkpointBody, func(cli *SessionClient) error {
			return cli.PostCheckpoint(t.Context(), "so-1", checkpoint)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			// Serve the write and check it is a signed POST with the body.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/sobject/so-1/"+tc.action {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("X-Signature") == "" {
					t.Error("missing X-Signature")
				}
				if body, _ := io.ReadAll(r.Body); !bytes.Equal(body, tc.body) {
					t.Errorf("body %q, want %q", body, tc.body)
				}
			}))
			defer srv.Close()

			// Post the write through a session client.
			priv, pid := generateTestKeypair(t)
			cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
			if err := tc.post(cli); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestPostOp_ServerError verifies PostOp returns error on server failure.
func TestPostOp_ServerError(t *testing.T) {
	// Serve every request with a 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer srv.Close()

	// Post an operation and require the failure.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
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

// TestGetSharedObjectDisplayName uses the cached shared object name.
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

// TestEnsureAccountSettingsSharedObject_AlreadyExists reuses the existing settings object.
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

// TestEnsureAccountSettingsSharedObject_UsesReadyBindingFromAccountState reuses the ready account binding.
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

// TestDeleteSharedObjectRemovesMetadataAndListCaches invalidates both object caches after deletion.
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

// TestCreateSharedObjectDeletesUninitializedObject deletes the catalog entry
// when the genesis state cannot be written.
func TestCreateSharedObjectDeletesUninitializedObject(t *testing.T) {
	// Accept the catalog create and delete, and reject every genesis write.
	const soID = "so-123"
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/create":
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/delete":
			deleted = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	// Create a Space and check that the failed create removed its entry.
	acc := NewTestProviderAccount(t, srv.URL)
	meta, err := space.NewSharedObjectMeta("Space")
	if err != nil {
		t.Fatalf("build shared object metadata: %v", err)
	}
	if _, err := acc.CreateSharedObject(context.Background(), soID, meta, "", ""); err == nil {
		t.Fatal("expected create to fail")
	}
	if !deleted {
		t.Fatal("expected the uninitialized shared object to be deleted")
	}
}

// TestFetchSharedObjectListPreservesCreatedCacheEntry retains locally created entries during list refresh.
func TestFetchSharedObjectListPreservesCreatedCacheEntry(t *testing.T) {
	const soID = "so-created"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{}))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE)
	meta, err := space.NewSharedObjectMeta("Created Space")
	if err != nil {
		t.Fatalf("NewSharedObjectMeta: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef(soID),
		Meta:   meta,
		Source: "created",
	})

	if err := acc.fetchSharedObjectList(context.Background()); err != nil {
		t.Fatalf("fetchSharedObjectList: %v", err)
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 1 {
		t.Fatalf("expected created shared object preserved, got %#v", list)
	}
	entry := list.GetSharedObjects()[0]
	if got := entry.GetRef().GetProviderResourceRef().GetId(); got != soID {
		t.Fatalf("expected cached SO id %q, got %q", soID, got)
	}
	if got := entry.GetSource(); got != "created" {
		t.Fatalf("expected source created, got %q", got)
	}
}

// TestEnsureSharedObjectListLoaded_NoSubscriptionSkipsFetch avoids fetching without account access.
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

// TestEnsureSharedObjectListLoaded_InvalidationRefetchesOnce coalesces invalidated list reads.
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

// TestGetAccountStateEnablesSharedObjectListAccess enables list reads after account access arrives.
func TestGetAccountStateEnablesSharedObjectListAccess(t *testing.T) {
	var listCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
			}))
		case "/api/sobject/list":
			listCalls++
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if acc.hasSharedObjectListAccess() {
		t.Fatal("expected new account to start without SO list access")
	}

	if _, err := acc.GetAccountState(context.Background()); err != nil {
		t.Fatalf("GetAccountState: %v", err)
	}
	if !acc.hasSharedObjectListAccess() {
		t.Fatal("expected account state fetch to enable SO list access")
	}

	if err := acc.RefreshSharedObjectList(context.Background()); err != nil {
		t.Fatalf("RefreshSharedObjectList: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected one shared object list fetch, got %d", listCalls)
	}
}

// TestRefreshSharedObjectListRefreshesAccountAccess refreshes account access before listing objects.
func TestRefreshSharedObjectListRefreshesAccountAccess(t *testing.T) {
	var accountStateCalls int
	var listCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			accountStateCalls++
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
			}))
		case "/api/sobject/list":
			listCalls++
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if acc.hasSharedObjectListAccess() {
		t.Fatal("expected new account to start without SO list access")
	}

	if err := acc.RefreshSharedObjectList(context.Background()); err != nil {
		t.Fatalf("RefreshSharedObjectList: %v", err)
	}
	if accountStateCalls != 1 {
		t.Fatalf("expected one account state fetch, got %d", accountStateCalls)
	}
	if listCalls != 1 {
		t.Fatalf("expected one shared object list fetch, got %d", listCalls)
	}
}

// TestGetSOState_Success verifies GetSOState sends GET to the correct path.
func TestGetSOState_Success(t *testing.T) {
	// Serve a state response.
	respBody := `{"checkpoint":{}}`
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

	// Build a client.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Fetching returns the response body.
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
