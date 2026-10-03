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
	// Prepare persistent storage and a helper that opens a fresh Session catalog.
	ctx := t.Context()
	stateRoot := t.TempDir()
	open := func() (*testbed.Testbed, *provider_local.Provider, session.SessionController, func()) {
		// Start the local account testbed over the persistent storage root.
		t.Helper()
		tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewBoltDB(false, stateRoot)))
		if err != nil {
			t.Fatal(err)
		}
		tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
		tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))

		// Mount the volume that stores the Session catalog.
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

		// Load the Session catalog on the mounted volume.
		_, catalogRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
			VolumeId: catalogVolume.GetID(),
		}), nil)
		if err != nil {
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}

		// Load the local provider with the testbed storage and peer.
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

		// Retain the local provider used to create and reopen the account.
		prov, lookupRef, err := provider.ExLookupProvider(ctx, tb.Bus, provider_local.ProviderID, false, nil)
		if err != nil {
			providerRef.Release()
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}

		// Retain the Session controller used to register the account.
		ctrl, ctrlRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
		if err != nil {
			lookupRef.Release()
			providerRef.Release()
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
			t.Fatal(err)
		}

		// Bind teardown to every retained controller and testbed reference.
		release := func() {
			// Release the Session catalog, provider, volume, and testbed together.
			ctrlRef.Release()
			lookupRef.Release()
			providerRef.Release()
			catalogRef.Release()
			volumeRef.Release()
			tb.Release()
		}
		return tb, prov.(*provider_local.Provider), ctrl, release
	}

	// Create the local account and its original Session key.
	_, prov, catalog, release := open()
	ref, err := prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		release()
		t.Fatal(err)
	}

	// Register the original Session in the first catalog.
	accountID := ref.GetProviderResourceRef().GetProviderAccountId()
	if _, err := catalog.RegisterSession(ctx, ref, nil); err != nil {
		release()
		t.Fatal(err)
	}

	// Open the local account while creating its persisted Space.
	account, releaseAccount, err := prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		release()
		t.Fatal(err)
	}

	// Create the Space metadata and persist the Space in the account.
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

	// Close the original account and remove only the Session catalog.
	releaseAccount()
	release()
	if err := os.Remove(filepath.Join(stateRoot, entrypoint_state.Filename)); err != nil {
		t.Fatalf("drop Session catalog: %v", err)
	}

	// The replacement state.s4wave has no entries; the account volume remains.
	reopenedTB, reopened, emptyCatalog, releaseReopened := open()
	defer releaseReopened()

	// Verify that the replacement Session catalog starts empty.
	before, err := emptyCatalog.ListSessions(ctx)
	if err != nil || len(before) != 0 {
		t.Fatalf("new catalog = %v, err %v; want empty", before, err)
	}

	// Attach the persisted account to the replacement Session catalog.
	server := &LocalProviderResource{b: reopenedTB.Bus, provider: reopened}
	request := &s4wave_provider_local.AttachAccountRequest{AccountId: accountID}
	first, err := server.AttachAccount(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	// Repeat account attachment and verify the Session remains unchanged.
	second, err := server.AttachAccount(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !first.GetSessionListEntry().EqualVT(second.GetSessionListEntry()) {
		t.Fatalf("repeat attachment changed Session: %v then %v", first, second)
	}

	// Verify that the catalog contains only the original Session.
	entries, err := emptyCatalog.ListSessions(ctx)
	if err != nil || len(entries) != 1 || !entries[0].GetSessionRef().EqualVT(ref) {
		t.Fatalf("reattached catalog = %v, err %v; want original Session", entries, err)
	}

	// Open the attached account to read its persisted Space inventory.
	attachedAccount, releaseAttached, err := reopened.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseAttached()

	// Wait for the attached account to publish its Space inventory.
	soList, err := attachedAccount.(*provider_local.ProviderAccount).GetSOListCtr().WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the recovered Space retains its identity and metadata.
	if !slices.ContainsFunc(soList.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().EqualVT(spaceRef) && entry.GetMeta().EqualVT(meta)
	}) {
		t.Fatalf("recovered Space missing from account catalog: %v", soList.GetSharedObjects())
	}

	// Mount the recovered Space from the attached account.
	mounted, releaseSpace, err := attachedAccount.(*provider_local.ProviderAccount).MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSpace()

	// Verify the mounted Space identity and read its persisted state.
	if mounted.GetSharedObjectID() != "recovered-space" {
		t.Fatalf("mounted Space = %q", mounted.GetSharedObjectID())
	}
	state, err := mounted.GetSharedObjectState(ctx)
	if err != nil || state == nil {
		t.Fatalf("read recovered Space state: %v", err)
	}
}
