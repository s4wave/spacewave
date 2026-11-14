package provider_spacewave_test

import (
	"context"
	"crypto/rand"
	"errors"
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

// serveTestAccountState answers the account state request the cloud target
// reads its owner's username from, and reports whether r was that request.
func serveTestAccountState(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/account/state" {
		return false
	}
	data, err := (&api.AccountStateResponse{AccountId: "test-account", EntityId: "alice"}).MarshalVT()
	if err != nil {
		t.Error(err)
	}
	_, _ = w.Write(data)
	return true
}

// buildTestTransferTarget creates a SpacewaveTransferTarget backed by a
// ProviderAccount pointing at the given test server URL.
func buildTestTransferTarget(t *testing.T, srvURL string) *provider_transfer.SpacewaveTransferTarget {
	t.Helper()
	acc := provider_spacewave.NewTestProviderAccount(t, srvURL)
	return provider_transfer.NewSpacewaveTransferTarget(acc, "spacewave", "test-account")
}

// TestCloudTransferTarget verifies that the spacewave transfer target creates
// a Space with its genesis config, checkpoint and transfer origin in one
// create-with-state request, and lists nothing before it.
func TestCloudTransferTarget(t *testing.T) {
	// Record what the fake cloud receives.
	var paths []string
	var created []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveTestAccountState(t, w, r) {
			return
		}
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/api/sobject/test-so/create-with-state" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		created, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Add the Space, which sends nothing.
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
	space, err := tgt.AddSharedObject(context.Background(), ref, meta, nil)
	if err != nil {
		t.Fatalf("AddSharedObject: %v", err)
	}
	if space.Holds != provider_transfer.TargetHoldsArriving || len(paths) != 0 {
		t.Fatalf("AddSharedObject: holds %v after %v, want arriving after nothing", space.Holds, paths)
	}

	// Write its state as a genesis owned by a new key.
	owner, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	origin := &provider_transfer.TransferOrigin{SourceProviderId: "local", SourceAccountId: "src", SourceHead: []byte("head")}
	if err := tgt.WriteSharedObjectState(context.Background(), le, "test-so", owner, []byte("world"), origin, space); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one create-with-state request, got %v", paths)
	}

	// The request carries the origin and the genesis checkpoint.
	req := &api.CreateWithStateRequest{}
	if err := req.UnmarshalVT(created); err != nil {
		t.Fatalf("unmarshal create-with-state request: %v", err)
	}
	sent := &provider_transfer.TransferOrigin{}
	if err := sent.UnmarshalVT(req.GetTransferOrigin()); err != nil || !sent.EqualVT(origin) {
		t.Fatalf("create-with-state carries origin %v, want %v", sent, origin)
	}
	checkpoint := &api.PostCheckpointRequest{}
	if err := checkpoint.UnmarshalVT(req.GetCheckpointState()); err != nil {
		t.Fatal(err)
	}
	inner, err := checkpoint.GetCheckpoint().UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	if inner.GetHeight() != 0 || inner.GetSharedObjectId() != "test-so" {
		t.Fatalf("posted checkpoint is not the genesis: %+v", inner)
	}

	// The genesis names the account's session as owner, with its account.
	config := &api.PostConfigStateRequest{}
	if err := config.UnmarshalVT(req.GetConfigState()); err != nil {
		t.Fatal(err)
	}
	genesis := &sobject.SOConfigChange{}
	if err := genesis.UnmarshalVT(config.GetConfigChange()); err != nil {
		t.Fatal(err)
	}
	owners := genesis.GetConfig().GetParticipants()
	if len(owners) != 1 || owners[0].GetPeerId() != tgt.GetAccount().GetCurrentSessionPeerID().String() || owners[0].GetEntityId() != "test-account" || owners[0].GetUsername() != "alice" {
		t.Fatalf("genesis owners %v, want the account's session", owners)
	}
}

// TestCloudTransferTargetRefusesListedSpace checks that the cloud target
// writes nothing over a Space the cloud holds: a create-with-state the cloud
// refuses, and a write no read chose, each return ErrTargetChanged.
func TestCloudTransferTargetRefusesListedSpace(t *testing.T) {
	// Answer every create with a conflict.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveTestAccountState(t, w, r) {
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/create-with-state") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()
	tgt := buildTestTransferTarget(t, srv.URL)

	// The create on a held id is refused.
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-so",
			ProviderId:        "spacewave",
			ProviderAccountId: "test-account",
		},
		BlockStoreId: "test-so",
	}
	meta := &sobject.SharedObjectMeta{BodyType: "space"}
	space, err := tgt.AddSharedObject(context.Background(), ref, meta, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	if err := tgt.WriteSharedObjectState(context.Background(), le, "test-so", owner, []byte("world"), nil, space); !errors.Is(err, provider_transfer.ErrTargetChanged) {
		t.Fatalf("WriteSharedObjectState: expected ErrTargetChanged, got %v", err)
	}

	// A write no read chose sends nothing.
	expect := &provider_transfer.TargetSpace{Holds: provider_transfer.TargetHoldsArriving}
	if err := tgt.WriteSharedObjectState(context.Background(), le, "test-so", owner, []byte("world"), nil, expect); !errors.Is(err, provider_transfer.ErrTargetChanged) {
		t.Fatalf("WriteSharedObjectState: expected ErrTargetChanged, got %v", err)
	}
}
