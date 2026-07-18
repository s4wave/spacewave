package provider_local

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
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
	accIface, accRel, err := localProv.AccessProviderAccount(
		ctx,
		"local-account-123",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer accRel()

	acc := accIface.(*ProviderAccount)
	if err := acc.writeLinkedCloudAccountID(ctx, "local-session-123", "cloud-account-123"); err != nil {
		t.Fatal(err)
	}
	cloudAccountID, err := acc.loadLinkedCloudAccountID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cloudAccountID != "cloud-account-123" {
		t.Fatalf("expected linked cloud account id, got %q", cloudAccountID)
	}
}

func TestFindLinkedCloudSessionRef(t *testing.T) {
	want := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "cloud-session",
			ProviderId:        "spacewave",
			ProviderAccountId: "cloud-account",
		},
	}
	entries := []*session.SessionListEntry{
		{
			SessionRef: &session.SessionRef{
				ProviderResourceRef: &provider.ProviderResourceRef{
					Id:                "local-session",
					ProviderId:        "local",
					ProviderAccountId: "cloud-account",
				},
			},
		},
		{
			SessionRef: &session.SessionRef{
				ProviderResourceRef: &provider.ProviderResourceRef{
					Id:                "other-cloud-session",
					ProviderId:        "spacewave",
					ProviderAccountId: "other-account",
				},
			},
		},
		{SessionRef: want},
	}

	got := findLinkedCloudSessionRef(entries, "cloud-account")
	if !got.EqualVT(want) {
		t.Fatalf("linked cloud session ref = %v, want %v", got, want)
	}
	if got := findLinkedCloudSessionRef(entries, "missing"); got != nil {
		t.Fatalf("missing linked cloud session ref = %v, want nil", got)
	}
}

func TestWaitForLinkedCloudSessionRef(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, sessionCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionCtrlRef.Release()

	ctrl, ctrlRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()

	type result struct {
		ref *session.SessionRef
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		ref, err := waitForLinkedCloudSessionRef(ctx, ctrl, "cloud-account")
		resultCh <- result{ref: ref, err: err}
	}()

	want := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "cloud-session",
			ProviderId:        "spacewave",
			ProviderAccountId: "cloud-account",
		},
	}
	if _, err := ctrl.RegisterSession(ctx, want, nil); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if !got.ref.EqualVT(want) {
			t.Fatalf("linked cloud session ref = %v, want %v", got.ref, want)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
