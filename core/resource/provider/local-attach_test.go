package resource_provider

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	"github.com/s4wave/spacewave/testbed"
)

// TestAttachAccountRecoversSpaceAfterCatalogLoss reopens a persisted local
// account through a fresh catalog and checks that its original Space is read.
func TestAttachAccountRecoversSpaceAfterCatalogLoss(t *testing.T) {
	ctx := t.Context()
	stateRoot := t.TempDir()
	open := func() (*testbed.Testbed, *provider_local.Provider, session.SessionController, func()) {
		t.Helper()
		tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewBoltDB(false, stateRoot)))
		if err != nil {
			t.Fatal(err)
		}
		tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
		tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
		volumeCtrl, volumeRef, err := storage_volume.ExecVolumeController(ctx, tb.Bus, entrypoint_state.NewVolumeConfig(tb.StorageID))
		if err != nil {
			tb.Release()
			t.Fatal(err)
		}
		catalogVolume, err := volumeCtrl.GetVolume(ctx)
		if err != nil {
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}
		_, catalogRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
			VolumeId: catalogVolume.GetID(),
		}), nil)
		if err != nil {
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}
		_, providerRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
			ProviderId: provider_local.ProviderID,
			PeerId:     tb.Volume.GetPeerID().String(),
			StorageId:  tb.StorageID,
		}), nil)
		if err != nil {
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}
		prov, lookupRef, err := provider.ExLookupProvider(ctx, tb.Bus, provider_local.ProviderID, false, nil)
		if err != nil {
			providerRef.Release()
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}
		ctrl, ctrlRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
		if err != nil {
			lookupRef.Release()
			providerRef.Release()
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}
		release := func() {
			ctrlRef.Release()
			lookupRef.Release()
			providerRef.Release()
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
		}
		return tb, prov.(*provider_local.Provider), ctrl, release
	}

	_, prov, catalog, release := open()
	ref, err := prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		release()
		t.Fatal(err)
	}
	accountID := ref.GetProviderResourceRef().GetProviderAccountId()
	if _, err := catalog.RegisterSession(ctx, ref, nil); err != nil {
		release()
		t.Fatal(err)
	}
	account, releaseAccount, err := prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		release()
		t.Fatal(err)
	}
	meta, err := space.NewSharedObjectMeta("Recovered Space")
	if err != nil {
		releaseAccount()
		release()
		t.Fatal(err)
	}
	spaceRef, err := account.(*provider_local.ProviderAccount).CreateSharedObject(ctx, "recovered-space", meta, "", "")
	if err != nil {
		releaseAccount()
		release()
		t.Fatal(err)
	}
	releaseAccount()
	release()
	if err := os.Remove(filepath.Join(stateRoot, entrypoint_state.Filename)); err != nil {
		t.Fatalf("drop Session catalog: %v", err)
	}

	// The replacement state.s4wave has no entries; the account volume remains.
	reopenedTB, reopened, emptyCatalog, releaseReopened := open()
	defer releaseReopened()
	before, err := emptyCatalog.ListSessions(ctx)
	if err != nil || len(before) != 0 {
		t.Fatalf("new catalog = %v, err %v; want empty", before, err)
	}
	server := &LocalProviderResource{b: reopenedTB.Bus, provider: reopened}
	request := &s4wave_provider_local.AttachAccountRequest{AccountId: accountID}
	first, err := server.AttachAccount(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.AttachAccount(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !first.GetSessionListEntry().EqualVT(second.GetSessionListEntry()) {
		t.Fatalf("repeat attachment changed Session: %v then %v", first, second)
	}
	entries, err := emptyCatalog.ListSessions(ctx)
	if err != nil || len(entries) != 1 || !entries[0].GetSessionRef().EqualVT(ref) {
		t.Fatalf("reattached catalog = %v, err %v; want original Session", entries, err)
	}
	attachedAccount, releaseAttached, err := reopened.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseAttached()
	soList, err := attachedAccount.(*provider_local.ProviderAccount).GetSOListCtr().WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(soList.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().EqualVT(spaceRef) && entry.GetMeta().EqualVT(meta)
	}) {
		t.Fatalf("recovered Space missing from account catalog: %v", soList.GetSharedObjects())
	}
	mounted, releaseSpace, err := attachedAccount.(*provider_local.ProviderAccount).MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSpace()
	if mounted.GetSharedObjectID() != "recovered-space" {
		t.Fatalf("mounted Space = %q", mounted.GetSharedObjectID())
	}
	state, err := mounted.GetSharedObjectState(ctx)
	if err != nil || state == nil {
		t.Fatalf("read recovered Space state: %v", err)
	}
}
