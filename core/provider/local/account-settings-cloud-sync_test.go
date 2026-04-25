package provider_local

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/testbed"
)

func TestBuildAccountSettingsSyncOps(t *testing.T) {
	source := &account_settings.AccountSettings{
		DisplayName: "Device A",
		PairedDevices: []*account_settings.PairedDevice{{
			PeerId:      "peer-a",
			DisplayName: "Laptop",
			PairedAt:    10,
		}},
		EntityKeypairs: []*session.EntityKeypair{{
			PeerId:     "kp-a",
			AuthMethod: "passkey",
		}},
	}
	target := &account_settings.AccountSettings{
		DisplayName: "Device B",
		PairedDevices: []*account_settings.PairedDevice{{
			PeerId:      "peer-b",
			DisplayName: "Old Phone",
			PairedAt:    1,
		}},
		EntityKeypairs: []*session.EntityKeypair{{
			PeerId:     "kp-b",
			AuthMethod: "pem",
		}},
	}

	ops, err := buildAccountSettingsSyncOps(source, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 5 {
		t.Fatalf("expected 5 sync ops, got %d", len(ops))
	}
}

func TestLoadLinkedCloudAccountID(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provCtrlRef.Release()

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	localProv := prov.(*Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "cloud-account-123")
	if err != nil {
		t.Fatal(err)
	}

	accIface, accRel, err := localProv.AccessProviderAccount(
		ctx,
		sessRef.GetProviderResourceRef().GetProviderAccountId(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer accRel()

	acc := accIface.(*ProviderAccount)
	cloudAccountID, err := acc.loadLinkedCloudAccountID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cloudAccountID != "cloud-account-123" {
		t.Fatalf("expected linked cloud account id, got %q", cloudAccountID)
	}
}
