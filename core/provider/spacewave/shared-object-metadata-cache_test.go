package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestGetSharedObjectMetadataSingleflightAndWarmCache(t *testing.T) {
	var hits atomic.Int32
	hitStarted := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/so-1/meta" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if hits.Add(1) == 1 {
			close(hitStarted)
		}
		<-release
		writeSharedObjectMetadata(t, w, &api.SpaceMetadataResponse{
			OwnerType:   "account",
			OwnerId:     "test-account",
			DisplayName: "Space One",
			ObjectType:  "space",
		})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	ctx := context.Background()
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			metadata, err := acc.GetSharedObjectMetadata(ctx, "so-1")
			if err != nil {
				errs <- err
				return
			}
			if metadata.GetDisplayName() != "Space One" {
				errs <- errorsUnexpectedMetadata(metadata.GetDisplayName())
			}
		})
	}
	<-hitStarted
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one cold seed, got %d", hits.Load())
	}

	metadata, err := acc.GetSharedObjectMetadata(ctx, "so-1")
	if err != nil {
		t.Fatalf("get warm metadata: %v", err)
	}
	metadata.DisplayName = "mutated"
	metadata, err = acc.GetSharedObjectMetadata(ctx, "so-1")
	if err != nil {
		t.Fatalf("get cloned warm metadata: %v", err)
	}
	if metadata.GetDisplayName() != "Space One" {
		t.Fatalf("expected clone-safe display name, got %q", metadata.GetDisplayName())
	}
	if hits.Load() != 1 {
		t.Fatalf("expected warm read to skip fetch, got %d hits", hits.Load())
	}
}

func TestGetSharedObjectMetadataInvalidationRefetch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/so-1/meta" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		hit := hits.Add(1)
		writeSharedObjectMetadata(t, w, &api.SpaceMetadataResponse{
			OwnerType:   "account",
			OwnerId:     "test-account",
			DisplayName: "Space " + strconv.Itoa(int(hit)),
			ObjectType:  "space",
		})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	ctx := context.Background()
	metadata, err := acc.GetSharedObjectMetadata(ctx, "so-1")
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if metadata.GetDisplayName() != "Space 1" {
		t.Fatalf("unexpected display name: %q", metadata.GetDisplayName())
	}

	acc.InvalidateSharedObjectMetadata("so-1")
	metadata, err = acc.GetSharedObjectMetadata(ctx, "so-1")
	if err != nil {
		t.Fatalf("get metadata after invalidation: %v", err)
	}
	if metadata.GetDisplayName() != "Space 2" {
		t.Fatalf("expected refreshed display name, got %q", metadata.GetDisplayName())
	}
	if hits.Load() != 2 {
		t.Fatalf("expected two seeds after invalidation, got %d", hits.Load())
	}
}

func TestSharedObjectMetadataDeleteTombstone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSharedObjectMetadata(t, w, &api.SpaceMetadataResponse{
			DisplayName: "Space One",
			ObjectType:  "space",
		})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if _, err := acc.GetSharedObjectMetadata(context.Background(), "so-1"); err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	acc.DeleteSharedObjectMetadata("so-1")
	if _, err := acc.GetSharedObjectMetadata(context.Background(), "so-1"); err != ErrSharedObjectMetadataDeleted {
		t.Fatalf("expected deleted metadata error, got %v", err)
	}
}

func TestSharedObjectMetadataInvalidationIsPerObject(t *testing.T) {
	hits := map[string]int{}
	var mtx sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mtx.Lock()
		hits[r.URL.Path]++
		hit := hits[r.URL.Path]
		mtx.Unlock()
		writeSharedObjectMetadata(t, w, &api.SpaceMetadataResponse{
			OwnerType:   "account",
			OwnerId:     "test-account",
			DisplayName: r.URL.Path + " " + strconv.Itoa(hit),
			ObjectType:  "space",
		})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	ctx := context.Background()
	if _, err := acc.GetSharedObjectMetadata(ctx, "so-1"); err != nil {
		t.Fatalf("get so-1 metadata: %v", err)
	}
	if _, err := acc.GetSharedObjectMetadata(ctx, "so-2"); err != nil {
		t.Fatalf("get so-2 metadata: %v", err)
	}
	acc.InvalidateSharedObjectMetadata("so-1")
	if _, err := acc.GetSharedObjectMetadata(ctx, "so-1"); err != nil {
		t.Fatalf("get invalidated so-1 metadata: %v", err)
	}
	metadata, err := acc.GetSharedObjectMetadata(ctx, "so-2")
	if err != nil {
		t.Fatalf("get warm so-2 metadata: %v", err)
	}
	if metadata.GetDisplayName() != "/api/sobject/so-2/meta 1" {
		t.Fatalf("expected warm so-2 metadata, got %q", metadata.GetDisplayName())
	}

	mtx.Lock()
	defer mtx.Unlock()
	if hits["/api/sobject/so-1/meta"] != 2 {
		t.Fatalf("expected so-1 to refetch once, got %d", hits["/api/sobject/so-1/meta"])
	}
	if hits["/api/sobject/so-2/meta"] != 1 {
		t.Fatalf("expected so-2 to stay warm, got %d", hits["/api/sobject/so-2/meta"])
	}
}

func TestSharedObjectMetadataReadPathsShareSeed(t *testing.T) {
	var hits atomic.Int32
	hitStarted := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/so-1/meta" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if hits.Add(1) == 1 {
			close(hitStarted)
		}
		<-release
		writeSharedObjectMetadata(t, w, &api.SpaceMetadataResponse{
			OwnerType:   "account",
			OwnerId:     "someone-else",
			DisplayName: "Space One",
			ObjectType:  "space",
		})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	enableTestCloudMutations(acc)
	ctx := context.Background()
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := acc.reconcilePendingParticipant(ctx, "so-1", "account-2"); err != nil {
			errs <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := acc.RepairSharedObject(ctx, "so-1"); err == nil {
			errs <- errors.New("expected repair authorization error")
		}
	}()

	<-hitStarted
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected independent cold read paths to share one seed, got %d", hits.Load())
	}

	if err := acc.reconcilePendingParticipant(ctx, "so-1", "account-2"); err != nil {
		t.Fatalf("warm pending participant reconciliation: %v", err)
	}
	if err := acc.RepairSharedObject(ctx, "so-1"); err == nil {
		t.Fatal("expected warm repair authorization error")
	}
	if hits.Load() != 1 {
		t.Fatalf("expected warm read paths to skip fetch, got %d hits", hits.Load())
	}
}

func TestUpdateSharedObjectMetadataSeedsCache(t *testing.T) {
	var updateHits atomic.Int32
	var metaHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/so-1/update":
			updateHits.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read update body: %v", err)
			}
			req := &api.SpaceMetadataResponse{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal update body: %v", err)
			}
			if req.GetDisplayName() != "Renamed Space" {
				t.Fatalf("unexpected update display name: %q", req.GetDisplayName())
			}
			writeSharedObjectMetadata(t, w, &api.SpaceMetadataResponse{
				OwnerType:   "account",
				OwnerId:     "test-account",
				DisplayName: "Renamed Space",
				ObjectType:  "space",
				PublicRead:  true,
			})
		case "/api/sobject/so-1/meta":
			metaHits.Add(1)
			t.Fatal("unexpected metadata fetch after update response")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	oldMeta, err := space.NewSharedObjectMeta("Old Space")
	if err != nil {
		t.Fatalf("build old metadata: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef("so-1"),
		Meta:   oldMeta,
		Source: "cloud",
	})
	metadata, err := acc.UpdateSharedObjectMetadata(
		context.Background(),
		"so-1",
		&api.SpaceMetadataResponse{DisplayName: "Renamed Space"},
	)
	if err != nil {
		t.Fatalf("update shared object metadata: %v", err)
	}
	if metadata.GetPublicRead() != true {
		t.Fatal("expected update response to include public read")
	}
	metadata.DisplayName = "mutated"
	metadata, err = acc.GetSharedObjectMetadata(context.Background(), "so-1")
	if err != nil {
		t.Fatalf("get cached metadata: %v", err)
	}
	if metadata.GetDisplayName() != "Renamed Space" {
		t.Fatalf("unexpected cached display name: %q", metadata.GetDisplayName())
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 1 {
		t.Fatalf("expected one cached shared object, got %#v", list)
	}
	if got := getSharedObjectDisplayName(list.GetSharedObjects()[0].GetMeta()); got != "Renamed Space" {
		t.Fatalf("unexpected patched list display name: %q", got)
	}
	if updateHits.Load() != 1 {
		t.Fatalf("expected one metadata update, got %d", updateHits.Load())
	}
	if metaHits.Load() != 0 {
		t.Fatalf("expected no metadata fetch, got %d", metaHits.Load())
	}
}

func enableTestCloudMutations(acc *ProviderAccount) {
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
}

func writeSharedObjectMetadata(
	t *testing.T,
	w http.ResponseWriter,
	metadata *api.SpaceMetadataResponse,
) {
	t.Helper()
	data, err := metadata.MarshalVT()
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func errorsUnexpectedMetadata(displayName string) error {
	return errors.Errorf("unexpected display name: %q", displayName)
}
