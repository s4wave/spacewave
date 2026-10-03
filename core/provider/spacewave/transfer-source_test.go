package provider_spacewave_test

import (
	"context"
	"crypto/rand"
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
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// buildTestTransferSource creates a SpacewaveTransferSource backed by a
// ProviderAccount pointing at the given test server URL.
func buildTestTransferSource(t *testing.T, srvURL string) *provider_transfer.SpacewaveTransferSource {
	t.Helper()
	acc := provider_spacewave.NewTestProviderAccount(t, srvURL)
	return provider_transfer.NewSpacewaveTransferSource(acc, "spacewave", "test-account", nil)
}

// TestCloudTransferSource verifies that the spacewave transfer source reads
// the shared object list from the cloud API.
func TestCloudTransferSource(t *testing.T) {
	soListData, err := (&sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{
		{
			Ref: &sobject.SharedObjectRef{
				ProviderResourceRef: &provider.ProviderResourceRef{Id: "so-1", ProviderAccountId: "test-account"},
				BlockStoreId:        "so-1",
			},
			Meta: &sobject.SharedObjectMeta{BodyType: "space"},
		},
		{
			Ref: &sobject.SharedObjectRef{
				ProviderResourceRef: &provider.ProviderResourceRef{Id: "so-2", ProviderId: "spacewave", ProviderAccountId: "test-account"},
				BlockStoreId:        "so-2",
			},
			Meta: &sobject.SharedObjectMeta{BodyType: "space"},
		},
	}}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal SO list: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sobject/list") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(soListData)
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
		prr := e.GetRef().GetProviderResourceRef()
		ids[prr.GetId()] = true
		if prr.GetProviderId() != "spacewave" {
			t.Fatalf("expected provider id filled in for %s, got %q", prr.GetId(), prr.GetProviderId())
		}
	}
	if !ids["so-1"] || !ids["so-2"] {
		t.Fatalf("expected so-1 and so-2, got %v", ids)
	}
}

// TestCloudTransferSourceEmptyList verifies empty SO list returns empty entries.
func TestCloudTransferSourceEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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
// shared objects and writes their genesis config and checkpoint to the cloud.
func TestCloudTransferTarget(t *testing.T) {
	// Record what the fake cloud receives.
	var createdID string
	var configPosted bool
	var postedCheckpoint []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/create"):
			createdID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/sobject/"), "/create")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/session/write-tickets/test-so":
			resp, err := (&api.WriteTicketBundleResponse{
				SoCheckpointTicket: "ticket-checkpoint",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal write ticket bundle: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(resp)
		case r.URL.Path == "/api/sobject/test-so/config-state":
			configPosted = true
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/sobject/test-so/checkpoint":
			if got := r.Header.Get("X-Write-Ticket"); got != "ticket-checkpoint" {
				t.Fatalf("unexpected write ticket: %q", got)
			}
			postedCheckpoint, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Target the fake cloud with a Space.
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

	// Create the shared object.
	err := tgt.AddSharedObject(context.Background(), ref, meta)
	if err != nil {
		t.Fatal(err)
	}
	if createdID != "test-so" {
		t.Fatalf("expected SO ID test-so, got %q", createdID)
	}

	// Write its state as a genesis owned by a new key.
	owner, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	if err := tgt.WriteSharedObjectState(context.Background(), le, "test-so", owner, []byte("world")); err != nil {
		t.Fatal(err)
	}
	if !configPosted {
		t.Fatal("expected the genesis config to be posted")
	}

	// The posted checkpoint is the genesis checkpoint.
	req := &api.PostCheckpointRequest{}
	if err := req.UnmarshalVT(postedCheckpoint); err != nil {
		t.Fatalf("unmarshal posted checkpoint request: %v", err)
	}
	inner, err := req.GetCheckpoint().UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	if inner.GetHeight() != 0 || inner.GetSharedObjectId() != "test-so" {
		t.Fatalf("posted checkpoint is not the genesis: %+v", inner)
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
