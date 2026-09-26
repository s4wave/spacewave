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

// startBoltLocalProvider starts a testbed and local provider whose accounts
// store their volumes in bolt databases under dir. The returned release stops
// both.
func startBoltLocalProvider(ctx context.Context, t *testing.T, dir string) (*provider_local.Provider, func()) {
	t.Helper()

	tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewBoltDB(false, dir)))
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

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
	t.Helper()

	acc, release, err := prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

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
// Each run starts a fresh bus on the same bolt directory. The second bus opens
// the account volume only after the first releases its file lock, so the
// account tracker restarts from stored state rather than reusing a live one.
func TestAccountSettingsBindingPersistsAcrossAccountRestart(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	prov, release := startBoltLocalProvider(ctx, t, dir)
	sessRef, err := prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		release()
		t.Fatal(err)
	}
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	id1 := accountSettingsID(ctx, t, prov, accountID)
	release()

	prov, release = startBoltLocalProvider(ctx, t, dir)
	defer release()
	if id2 := accountSettingsID(ctx, t, prov, accountID); id2 != id1 {
		t.Fatalf("expected account settings id %q after account restart, got %q", id1, id2)
	}
}
