package account_settings

import (
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

// testStorageBackend returns a complete storage backend.
func testStorageBackend(id, name string) *StorageBackend {
	return &StorageBackend{
		Id:          id,
		DisplayName: name,
		S3: &S3Location{
			Endpoint: "127.0.0.1:9000",
			Bucket:   "spacewave",
		},
		Credential: &s4wave_secret.Secret{
			Kind: s4wave_secret.SecretKindStorageCredential,
			Ref: &sobject.SharedObjectRef{
				ProviderResourceRef: &provider.ProviderResourceRef{
					Id:                "secret-" + id,
					ProviderId:        "local",
					ProviderAccountId: "account",
				},
				BlockStoreId: "secret-" + id,
			},
		},
	}
}

// applyOp marshals and applies one AccountSettingsOp.
func applyOp(t *testing.T, s *AccountSettings, op *AccountSettingsOp) error {
	t.Helper()
	data, err := op.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return s.applyOpData(data)
}

// TestStorageBackendOps checks the storage backend, default, and placement
// rules together.
func TestStorageBackendOps(t *testing.T) {
	s := &AccountSettings{}
	upsert := func(b *StorageBackend) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_UpsertStorageBackend{UpsertStorageBackend: b}})
	}
	remove := func(id string) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_RemoveStorageBackend{
			RemoveStorageBackend: &RemoveStorageBackendOp{StorageBackendId: id},
		}})
	}
	setDefault := func(id string) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_SetDefaultStorageBackend{
			SetDefaultStorageBackend: &SetDefaultStorageBackendOp{StorageBackendId: id},
		}})
	}
	place := func(blockStoreID, backendID string) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_SetBlockStorePlacement{
			SetBlockStorePlacement: &BlockStorePlacement{BlockStoreId: blockStoreID, StorageBackendId: backendID},
		}})
	}
	completeRelease := func(blockStoreID, backendID string) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_CompleteStorageRelease{
			CompleteStorageRelease: &BlockStorePlacement{BlockStoreId: blockStoreID, StorageBackendId: backendID},
		}})
	}
	expectReleases := func(want ...string) {
		t.Helper()
		var got []string
		for _, release := range s.GetStorageReleases() {
			got = append(got, release.GetBlockStoreId()+"@"+release.GetStorageBackendId())
		}
		if !slices.Equal(got, want) {
			t.Fatalf("expected releases %v, got %v", want, got)
		}
	}

	// Add two backends; a duplicate name and an incomplete backend fail.
	if err := upsert(testStorageBackend("a", "minio")); err != nil {
		t.Fatal(err)
	}
	if err := upsert(testStorageBackend("b", "r2")); err != nil {
		t.Fatal(err)
	}
	if err := upsert(testStorageBackend("c", "minio")); err == nil {
		t.Fatal("expected duplicate display name to fail")
	}
	incomplete := testStorageBackend("d", "b2")
	incomplete.S3.Bucket = ""
	if err := upsert(incomplete); err == nil {
		t.Fatal("expected a backend without a bucket to fail")
	}

	// Renaming a backend replaces it in place.
	if err := upsert(testStorageBackend("a", "home minio")); err != nil {
		t.Fatal(err)
	}
	if n := len(s.GetStorageBackends()); n != 2 {
		t.Fatalf("expected 2 backends, got %d", n)
	}
	if s.FindStorageBackendByName("home minio").GetId() != "a" {
		t.Fatal("expected the renamed backend")
	}

	// The default and placements must name a known backend.
	if err := setDefault("missing"); !errors.Is(err, ErrStorageBackendNotFound) {
		t.Fatalf("expected ErrStorageBackendNotFound, got %v", err)
	}
	if err := place("space-1", "missing"); !errors.Is(err, ErrStorageBackendNotFound) {
		t.Fatalf("expected ErrStorageBackendNotFound, got %v", err)
	}
	if err := setDefault("a"); err != nil {
		t.Fatal(err)
	}
	if err := place("space-1", "a"); err != nil {
		t.Fatal(err)
	}
	if err := place("space-1", "b"); err != nil {
		t.Fatal(err)
	}
	if got := s.FindBlockStorePlacement("space-1").GetStorageBackendId(); got != "b" {
		t.Fatalf("expected space-1 on b, got %q", got)
	}
	expectReleases("space-1@a")

	// A backend holding a block store cannot be removed until it is empty.
	if err := remove("b"); !errors.Is(err, ErrStorageBackendInUse) {
		t.Fatalf("expected ErrStorageBackendInUse, got %v", err)
	}
	if err := place("space-1", ""); err != nil {
		t.Fatal(err)
	}
	if s.FindBlockStorePlacement("space-1") != nil {
		t.Fatal("expected space-1 back on the account's storage")
	}
	expectReleases("space-1@a", "space-1@b")

	// Returning to a backend cancels its pending release.
	if err := place("space-1", "b"); err != nil {
		t.Fatal(err)
	}
	expectReleases("space-1@a")

	// Deleting a placed Space releases its placement and its objects.
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "space-1",
			ProviderId:        "local",
			ProviderAccountId: "account",
		},
		BlockStoreId: "space-1",
	}
	if err := applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_UpsertCatalogEntry{
		UpsertCatalogEntry: &AccountCatalogEntry{
			Entry:   &sobject.SharedObjectListEntry{Ref: ref, Meta: &sobject.SharedObjectMeta{BodyType: "space"}},
			Deleted: true,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if s.FindBlockStorePlacement("space-1") != nil {
		t.Fatal("expected the deleted Space's placement released")
	}
	expectReleases("space-1@a", "space-1@b")

	// Removing a backend drops its pending releases; completing one removes it.
	if err := remove("b"); err != nil {
		t.Fatal(err)
	}
	expectReleases("space-1@a")
	if err := completeRelease("space-1", ""); err == nil {
		t.Fatal("expected a release without a backend to fail")
	}
	if err := completeRelease("space-1", "a"); err != nil {
		t.Fatal(err)
	}
	expectReleases()

	// Removing the default backend clears the default.
	if err := remove("a"); err != nil {
		t.Fatal(err)
	}
	if s.GetDefaultStorageBackendId() != "" || len(s.GetStorageBackends()) != 0 {
		t.Fatalf("expected no backends and no default, got %v", s)
	}
}
