package provider_local_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/testbed"
)

// TestAccountSettingsBindingBootstrap verifies the local provider persists a
// bound unique-id account settings ref when the account first initializes.
func TestAccountSettingsBindingBootstrap(t *testing.T) {
	ctx := t.Context()

	_, _, acc, _, release := setupProviderAndSession(ctx, t)
	defer release()

	ref, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetProviderResourceRef().GetId() == account_settings.BindingPurpose {
		t.Fatalf("expected unique account settings id, got binding purpose %q", account_settings.BindingPurpose)
	}

	soList := acc.GetSOListCtr().GetValue()
	if soList == nil {
		t.Fatal("shared object list is nil")
	}

	var found bool
	for _, entry := range soList.GetSharedObjects() {
		entryRef := entry.GetRef()
		if entryRef.GetProviderResourceRef().GetId() != ref.GetProviderResourceRef().GetId() {
			continue
		}
		if entry.GetMeta().GetBodyType() != account_settings.BodyType {
			t.Fatalf("expected body type %q, got %q", account_settings.BodyType, entry.GetMeta().GetBodyType())
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("bound account settings SO %q not found in shared object list", ref.GetProviderResourceRef().GetId())
	}
}

func startLocalProvider(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
) (*provider_local.Provider, func()) {
	t.Helper()

	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		provCtrlRef.Release()
		t.Fatal(err)
	}

	return prov.(*provider_local.Provider), func() {
		provRef.Release()
		provCtrlRef.Release()
	}
}

// TestAccountSettingsBindingPersistsAcrossAccountRestart verifies the local
// binding survives a provider-account tracker restart with no cloud account.
func TestAccountSettingsBindingPersistsAcrossAccountRestart(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	localProv, releaseProv := startLocalProvider(ctx, t, tb)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		releaseProv()
		t.Fatal(err)
	}

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		releaseProv()
		t.Fatal(err)
	}
	ref1, err := accIface.(*provider_local.ProviderAccount).GetAccountSettingsRef(ctx)
	if err != nil {
		accRel()
		releaseProv()
		t.Fatal(err)
	}
	accRel()

	accIface, accRel, err = localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		releaseProv()
		t.Fatal(err)
	}
	defer accRel()
	defer releaseProv()

	ref2, err := accIface.(*provider_local.ProviderAccount).GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref2.GetProviderResourceRef().GetId() != ref1.GetProviderResourceRef().GetId() {
		t.Fatalf(
			"expected account settings id %q after account restart, got %q",
			ref1.GetProviderResourceRef().GetId(),
			ref2.GetProviderResourceRef().GetId(),
		)
	}
}
