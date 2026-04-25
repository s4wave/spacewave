package account_settings_test

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// setupProviderAccount creates a testbed with a local provider and account.
// Returns the testbed, session ref, account ID, provider account, and release function.
func setupProviderAccount(ctx context.Context, t *testing.T) (*testbed.Testbed, *session.SessionRef, string, *provider_local.ProviderAccount, func()) {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	providerID := "local"
	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	acc := accIface.(*provider_local.ProviderAccount)
	release := func() {
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
	return tb, sessRef, accountID, acc, release
}

// mountAccountSettingsSO mounts the account settings SO via the bus.
func mountAccountSettingsSO(ctx context.Context, t *testing.T, b bus.Bus, accountID string) (sobject.SharedObject, func()) {
	t.Helper()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	accIface, accRel, err := prov.(*provider_local.Provider).AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer accRel()

	ref, err := accIface.(*provider_local.ProviderAccount).GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}

	so, mountRef, err := sobject.ExMountSharedObject(ctx, b, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return so, func() { mountRef.Release() }
}

// decodeAccountSettings decodes AccountSettings from a SharedObjectStateSnapshot.
func decodeAccountSettings(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*account_settings.AccountSettings, error) {
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, err
	}
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	return settings, nil
}

// queueOpAndWaitState queues an operation and watches the state until the
// validator function returns true. This avoids the WaitOperation race when
// the processor goroutine runs concurrently.
func queueOpAndWaitState(
	ctx context.Context,
	t *testing.T,
	so sobject.SharedObject,
	opData []byte,
	valid func(settings *account_settings.AccountSettings) bool,
) {
	t.Helper()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	_, err = so.QueueOperation(ctx, opData)
	if err != nil {
		t.Fatal(err)
	}

	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			settings, err := decodeAccountSettings(ctx, snap)
			if err != nil {
				return err
			}
			if valid(settings) {
				return io.EOF
			}
			return nil
		},
		nil,
	)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
}

// TestAccountSettingsSOCreate verifies that the account settings SO is
// automatically created when a ProviderAccount initializes.
func TestAccountSettingsSOCreate(t *testing.T) {
	ctx := context.Background()

	tb, _, accountID, acc, release := setupProviderAccount(ctx, t)
	defer release()

	// Get the SO provider feature to access the list.
	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}

	// Access the shared object list.
	soListCtr, soListRel, err := soProv.AccessSharedObjectList(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer soListRel()

	soList := soListCtr.GetValue()
	if soList == nil {
		t.Fatal("shared object list is nil")
	}

	ref, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetProviderResourceRef().GetId() == account_settings.BindingPurpose {
		t.Fatalf("expected unique account settings id, got binding purpose %q", account_settings.BindingPurpose)
	}

	// The account settings SO should be present under the bound ref.
	var foundRef *sobject.SharedObjectRef
	for _, entry := range soList.GetSharedObjects() {
		entryRef := entry.GetRef()
		if entryRef.GetProviderResourceRef().GetId() == ref.GetProviderResourceRef().GetId() {
			if entry.GetMeta().GetBodyType() != account_settings.BodyType {
				t.Fatalf("expected body type %q, got %q", account_settings.BodyType, entry.GetMeta().GetBodyType())
			}
			foundRef = entryRef
			break
		}
	}
	if foundRef == nil {
		t.Fatalf("account settings SO %q not found in shared object list", ref.GetProviderResourceRef().GetId())
	}

	// Mount the account settings SO and verify it's readable.
	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	state, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	rootInner, err := state.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if rootInner.GetSeqno() == 0 {
		t.Fatal("expected seqno > 0")
	}
}

// TestPairedDeviceCRUD verifies adding and removing paired devices via SO operations.
func TestPairedDeviceCRUD(t *testing.T) {
	ctx := context.Background()

	tb, _, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	// Add a paired device and wait for state to reflect it.
	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      "12D3KooWTestPeer1",
				DisplayName: "Test Device 1",
				PairedAt:    1000,
			},
		},
	}
	addOpData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOpData, func(s *account_settings.AccountSettings) bool {
		return len(s.GetPairedDevices()) == 1
	})

	// Read the state and verify the device details.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	snap := stateCtr.GetValue()
	relStateCtr()

	settings, err := decodeAccountSettings(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	dev := settings.GetPairedDevices()[0]
	if dev.GetPeerId() != "12D3KooWTestPeer1" {
		t.Fatalf("expected peer_id %q, got %q", "12D3KooWTestPeer1", dev.GetPeerId())
	}
	if dev.GetDisplayName() != "Test Device 1" {
		t.Fatalf("expected display_name %q, got %q", "Test Device 1", dev.GetDisplayName())
	}

	// Add a second device.
	addOp2 := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      "12D3KooWTestPeer2",
				DisplayName: "Test Device 2",
				PairedAt:    2000,
			},
		},
	}
	addOp2Data, err := addOp2.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOp2Data, func(s *account_settings.AccountSettings) bool {
		return len(s.GetPairedDevices()) == 2
	})

	// Remove the first device.
	rmOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemovePairedDevice{
			RemovePairedDevice: &account_settings.RemovePairedDeviceOp{
				PeerId: "12D3KooWTestPeer1",
			},
		},
	}
	rmOpData, err := rmOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, rmOpData, func(s *account_settings.AccountSettings) bool {
		if len(s.GetPairedDevices()) != 1 {
			return false
		}
		return s.GetPairedDevices()[0].GetPeerId() == "12D3KooWTestPeer2"
	})
}

// TestSessionPresentationCRUD verifies adding and removing mirrored session
// presentation metadata via account-settings SO operations.
func TestSessionPresentationCRUD(t *testing.T) {
	ctx := context.Background()

	tb, _, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	upsert := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     "12D3KooWSessionPeer1",
				Label:      "Chrome on macOS (Portland, OR)",
				DeviceType: "web",
				ClientName: "Chrome",
				Os:         "macOS",
				Location:   "Portland, OR",
			},
		},
	}
	upsertData, err := upsert.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueOpAndWaitState(ctx, t, so, upsertData, func(settings *account_settings.AccountSettings) bool {
		presentations := settings.GetSessionPresentations()
		return len(presentations) == 1 &&
			presentations[0].GetPeerId() == "12D3KooWSessionPeer1" &&
			presentations[0].GetClientName() == "Chrome"
	})

	remove := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveSessionPresentation{
			RemoveSessionPresentation: &account_settings.RemoveSessionPresentationOp{
				PeerId: "12D3KooWSessionPeer1",
			},
		},
	}
	removeData, err := remove.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueOpAndWaitState(ctx, t, so, removeData, func(settings *account_settings.AccountSettings) bool {
		return len(settings.GetSessionPresentations()) == 0
	})
}

// TestEntityKeypairCRUD verifies adding and removing entity keypairs via SO operations.
func TestEntityKeypairCRUD(t *testing.T) {
	ctx := context.Background()

	tb, _, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	// Add an entity keypair.
	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     "12D3KooWKeypair1",
				AuthMethod: "password",
			},
		},
	}
	addOpData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOpData, func(s *account_settings.AccountSettings) bool {
		return len(s.GetEntityKeypairs()) == 1
	})

	// Verify the keypair details.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	snap := stateCtr.GetValue()
	relStateCtr()

	settings, err := decodeAccountSettings(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	kp := settings.GetEntityKeypairs()[0]
	if kp.GetPeerId() != "12D3KooWKeypair1" {
		t.Fatalf("expected peer_id %q, got %q", "12D3KooWKeypair1", kp.GetPeerId())
	}
	if kp.GetAuthMethod() != "password" {
		t.Fatalf("expected auth_method %q, got %q", "password", kp.GetAuthMethod())
	}

	// Add a second keypair.
	addOp2 := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     "12D3KooWKeypair2",
				AuthMethod: "pem",
			},
		},
	}
	addOp2Data, err := addOp2.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOp2Data, func(s *account_settings.AccountSettings) bool {
		return len(s.GetEntityKeypairs()) == 2
	})

	// Add duplicate (same peer_id) - should deduplicate.
	addOp3 := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     "12D3KooWKeypair1",
				AuthMethod: "password",
			},
		},
	}
	addOp3Data, err := addOp3.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOp3Data, func(s *account_settings.AccountSettings) bool {
		for _, k := range s.GetEntityKeypairs() {
			if k.GetPeerId() == "12D3KooWKeypair1" && k.GetAuthMethod() == "password" {
				return true
			}
		}
		return false
	})

	// Remove the first keypair.
	rmOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveEntityKeypair{
			RemoveEntityKeypair: &account_settings.RemoveEntityKeypairOp{
				PeerId: "12D3KooWKeypair1",
			},
		},
	}
	rmOpData, err := rmOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, rmOpData, func(s *account_settings.AccountSettings) bool {
		if len(s.GetEntityKeypairs()) != 1 {
			return false
		}
		return s.GetEntityKeypairs()[0].GetPeerId() == "12D3KooWKeypair2"
	})
}

// TestListPairedDevices verifies the WatchPairedDevices RPC on the session
// resource returns paired devices from the account settings SO.
func TestListPairedDevices(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	// Mount the session.
	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	// Create the session resource (RPC handler).
	le := logrus.NewEntry(logrus.StandardLogger())
	sr := resource_session.NewSessionResource(le, tb.Bus, sess)

	// Add a paired device via SO operations.
	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      "12D3KooWTestPeer1",
				DisplayName: "Test Device 1",
				PairedAt:    1000,
			},
		},
	}
	addOpData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOpData, func(s *account_settings.AccountSettings) bool {
		return len(s.GetPairedDevices()) == 1
	})

	// Call WatchPairedDevices and verify it returns the device.
	// Use a context that cancels after the first response.
	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	var received *s4wave_session.WatchPairedDevicesResponse
	strm := &testWatchPairedDevicesStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_session.WatchPairedDevicesResponse) error {
			if len(resp.GetPairedDevices()) > 0 {
				received = resp
				rpcCancel()
			}
			return nil
		},
	}

	err = sr.WatchPairedDevices(&s4wave_session.WatchPairedDevicesRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	if received == nil {
		t.Fatal("expected WatchPairedDevices to return paired devices")
	}
	if len(received.GetPairedDevices()) != 1 {
		t.Fatalf("expected 1 paired device, got %d", len(received.GetPairedDevices()))
	}
	dev := received.GetPairedDevices()[0]
	if dev.GetPeerId() != "12D3KooWTestPeer1" {
		t.Fatalf("expected peer_id %q, got %q", "12D3KooWTestPeer1", dev.GetPeerId())
	}
	if dev.GetDisplayName() != "Test Device 1" {
		t.Fatalf("expected display_name %q, got %q", "Test Device 1", dev.GetDisplayName())
	}
}

// TestLocalSessionAddEntityKeypair verifies the local session resource writes
// entity keypairs through the bound account settings ref.
func TestLocalSessionAddEntityKeypair(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	lsr := resource_session.NewLocalSessionResource(tb.Bus, sess)
	resp, err := lsr.AddEntityKeypair(ctx, &s4wave_session.AddLocalEntityKeypairRequest{
		Credential: &session.EntityCredential{
			Credential: &session.EntityCredential_Password{Password: "test-password"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetPeerId() == "" {
		t.Fatal("expected added entity keypair peer id")
	}

	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	var settings *account_settings.AccountSettings
	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			var err error
			settings, err = decodeAccountSettings(ctx, snap)
			if err != nil {
				return err
			}
			if len(settings.GetEntityKeypairs()) == 1 {
				return io.EOF
			}
			return nil
		},
		nil,
	)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if settings == nil {
		t.Fatal("expected account settings state")
	}
	if len(settings.GetEntityKeypairs()) != 1 {
		t.Fatalf("expected 1 entity keypair, got %d", len(settings.GetEntityKeypairs()))
	}
	kp := settings.GetEntityKeypairs()[0]
	if kp.GetPeerId() != resp.GetPeerId() {
		t.Fatalf("expected peer id %q, got %q", resp.GetPeerId(), kp.GetPeerId())
	}
	if kp.GetAuthMethod() != "password" {
		t.Fatalf("expected auth method %q, got %q", "password", kp.GetAuthMethod())
	}
}

// testWatchPairedDevicesStream is a mock stream for testing WatchPairedDevices.
type testWatchPairedDevicesStream struct {
	ctx    context.Context
	onSend func(*s4wave_session.WatchPairedDevicesResponse) error
}

func (s *testWatchPairedDevicesStream) Context() context.Context     { return s.ctx }
func (s *testWatchPairedDevicesStream) MsgRecv(_ srpc.Message) error { return nil }
func (s *testWatchPairedDevicesStream) CloseSend() error             { return nil }
func (s *testWatchPairedDevicesStream) Close() error                 { return nil }
func (s *testWatchPairedDevicesStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchPairedDevicesStream) Send(resp *s4wave_session.WatchPairedDevicesResponse) error {
	return s.onSend(resp)
}

func (s *testWatchPairedDevicesStream) SendAndClose(resp *s4wave_session.WatchPairedDevicesResponse) error {
	return s.onSend(resp)
}
