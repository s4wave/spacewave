//go:build !js

package provider_local_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/testbed"
)

// startLocalProvider starts a testbed and local provider whose accounts
// store their volumes in s4db files under dir. The returned release stops
// both.
func startLocalProvider(ctx context.Context, t *testing.T, dir string) (*provider_local.Provider, func()) {
	// Mark this helper for test failure attribution.
	t.Helper()

	// Open an s4db-backed testbed and register the local provider factory.
	tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewS4db(false, dir)))
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	// Load the local provider controller.
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	// Look up the local provider and release the testbed with it.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	return prov.(*provider_local.Provider), func() {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
}

// accountSettingsID returns the id of the account's bound settings object.
func accountSettingsID(ctx context.Context, t *testing.T, prov *provider_local.Provider, accountID string) string {
	// Mark this helper for test failure attribution.
	t.Helper()

	// Open the account and release it when the id is read.
	acc, release, err := prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Return the bound account-settings object id.
	ref, err := acc.(*provider_local.ProviderAccount).GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ref.GetProviderResourceRef().GetId()
}

// TestAccountSettingsBindingPersistsAcrossAccountRestart verifies the local
// binding survives a restart of the provider and its account with no cloud
// account.
//
// Each run starts a fresh bus on the same storage directory. The second bus opens
// the account volume only after the first releases its file lock, so the
// account tracker restarts from stored state rather than reusing a live one.
func TestAccountSettingsBindingPersistsAcrossAccountRestart(t *testing.T) {
	// Use a temporary storage directory for the restart.
	ctx := t.Context()
	dir := t.TempDir()

	// Create an account and record its settings id before shutdown.
	prov, release := startLocalProvider(ctx, t, dir)
	sessRef, err := prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		release()
		t.Fatal(err)
	}
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	id1 := accountSettingsID(ctx, t, prov, accountID)
	release()

	// Restart the provider and require the same settings id.
	prov, release = startLocalProvider(ctx, t, dir)
	defer release()
	if id2 := accountSettingsID(ctx, t, prov, accountID); id2 != id1 {
		t.Fatalf("expected account settings id %q after account restart, got %q", id1, id2)
	}
}
