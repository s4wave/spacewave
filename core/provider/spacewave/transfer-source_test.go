package provider_spacewave_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	"github.com/s4wave/spacewave/core/sobject"
)

// buildTestTransferSource creates a SpacewaveTransferSource backed by a
// ProviderAccount pointing at the given test server URL.
func buildTestTransferSource(t *testing.T, srvURL string) *provider_transfer.SpacewaveTransferSource {
	t.Helper()
	acc := provider_spacewave.NewTestProviderAccount(t, srvURL)
	return provider_transfer.NewSpacewaveTransferSource(acc, "spacewave", "test-account")
}

// TestCloudTransferSource verifies that the spacewave transfer source reads
// the shared object list from the cloud API.
func TestCloudTransferSource(t *testing.T) {
	soListJSON := `{"sharedObjects":[` +
		`{"ref":{"providerResourceRef":{"id":"so-1","providerId":"spacewave","providerAccountId":"test-account"},"blockStoreId":"so-1"},"meta":{"bodyType":"space"}},` +
		`{"ref":{"providerResourceRef":{"id":"so-2","providerId":"spacewave","providerAccountId":"test-account"},"blockStoreId":"so-2"},"meta":{"bodyType":"space"}}` +
		`]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sobject/list") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(soListJSON))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	src := buildTestTransferSource(t, srv.URL)

	list, err := src.GetSharedObjectList(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	entries := list.GetSharedObjects()
	if len(entries) != 2 {
		t.Fatalf("expected 2 shared objects, got %d", len(entries))
	}

	ids := make(map[string]bool)
	for _, e := range entries {
		ids[e.GetRef().GetProviderResourceRef().GetId()] = true
	}
	if !ids["so-1"] || !ids["so-2"] {
		t.Fatalf("expected so-1 and so-2, got %v", ids)
	}
}

// TestCloudTransferSourceSOState verifies that the spacewave transfer source
// reads SO state from the cloud API.
func TestCloudTransferSourceSOState(t *testing.T) {
	stateJSON, err := (&sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 1},
	}).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal SO state: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/state") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(stateJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	src := buildTestTransferSource(t, srv.URL)

	state, err := src.GetSharedObjectState(context.Background(), "so-1")
	if err != nil {
		t.Fatal(err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.GetRoot().GetInnerSeqno() != 1 {
		t.Fatalf("expected inner seqno 1, got %d", state.GetRoot().GetInnerSeqno())
	}
}

// TestCloudTransferSourceEmptyList verifies empty SO list returns empty entries.
func TestCloudTransferSourceEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sharedObjects":[]}`))
	}))
	defer srv.Close()

	src := buildTestTransferSource(t, srv.URL)

	list, err := src.GetSharedObjectList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetSharedObjects()) != 0 {
		t.Fatalf("expected 0 shared objects, got %d", len(list.GetSharedObjects()))
	}
}

// buildTestTransferTarget creates a SpacewaveTransferTarget backed by a
// ProviderAccount pointing at the given test server URL.
func buildTestTransferTarget(t *testing.T, srvURL string) *provider_transfer.SpacewaveTransferTarget {
	t.Helper()
	acc := provider_spacewave.NewTestProviderAccount(t, srvURL)
	return provider_transfer.NewSpacewaveTransferTarget(acc, "spacewave", "test-account")
}

// TestCloudTransferTarget verifies that the spacewave transfer target creates
// shared objects and writes state to the cloud.
func TestCloudTransferTarget(t *testing.T) {
	var createdID string
	var statePosted bool
	var postedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/create"):
			createdID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/sobject/"), "/create")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/session/write-tickets/test-so":
			resp, err := (&api.WriteTicketBundleResponse{
				SoRootTicket: "ticket-root",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal write ticket bundle: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(resp)
		case strings.Contains(r.URL.Path, "/root"):
			if got := r.Header.Get("X-Write-Ticket"); got != "ticket-root" {
				t.Fatalf("unexpected write ticket: %q", got)
			}
			statePosted = true
			postedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tgt := buildTestTransferTarget(t, srv.URL)

	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-so",
			ProviderId:        "spacewave",
			ProviderAccountId: "test-account",
		},
		BlockStoreId: "test-so",
	}
	meta := &sobject.SharedObjectMeta{BodyType: "space"}

	// Test AddSharedObject
	err := tgt.AddSharedObject(context.Background(), ref, meta)
	if err != nil {
		t.Fatal(err)
	}
	if createdID != "test-so" {
		t.Fatalf("expected SO ID test-so, got %q", createdID)
	}

	// Test WriteSharedObjectState
	state := &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	err = tgt.WriteSharedObjectState(context.Background(), "test-so", state)
	if err != nil {
		t.Fatal(err)
	}
	if !statePosted {
		t.Fatal("expected state to be posted")
	}
	if len(postedBody) == 0 {
		t.Fatal("expected non-empty state body")
	}

	// Verify the posted body can be unmarshaled back.
	req := &api.PostRootRequest{}
	if err := req.UnmarshalVT(postedBody); err != nil {
		t.Fatalf("unmarshal posted root request: %v", err)
	}
	if req.GetRoot().GetInnerSeqno() != 1 {
		t.Fatalf("expected inner seqno 1, got %d", req.GetRoot().GetInnerSeqno())
	}
}

func TestCloudTransferTargetAddSharedObjectAlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/create") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	tgt := buildTestTransferTarget(t, srv.URL)
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-so",
			ProviderId:        "spacewave",
			ProviderAccountId: "test-account",
		},
		BlockStoreId: "test-so",
	}
	meta := &sobject.SharedObjectMeta{BodyType: "space"}

	if err := tgt.AddSharedObject(context.Background(), ref, meta); err != nil {
		t.Fatalf("AddSharedObject: %v", err)
	}
}
