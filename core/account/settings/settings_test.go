package account_settings_test

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	"github.com/s4wave/spacewave/testbed"
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
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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

// TestKeybindingOverrideCRUD verifies account-scoped keybinding override ops
// validate command IDs, replace duplicate command rows, reject malformed ops,
// and leave existing state intact on rejection.
func TestKeybindingOverrideCRUD(t *testing.T) {
	ctx := t.Context()
	peerID := "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"
	initial := &account_settings.AccountSettings{
		KeybindingOverrides: &s4wave_command.KeybindingOverrideSet{
			Version: 1,
			Overrides: []*s4wave_command.KeybindingCommandOverride{{
				CommandId: "spacewave.existing",
				Bindings: []*s4wave_command.CommandBinding{{
					Id: "existing-default",
					Binding: &s4wave_command.CommandBinding_Combo{
						Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+E"},
					},
					When: s4wave_command.CommandFocusContext_COMMAND_FOCUS_CONTEXT_GLOBAL,
				}},
			}},
		},
	}
	currentData, err := initial.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	staleUpsert, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertKeybindingOverride{
			UpsertKeybindingOverride: &s4wave_command.KeybindingCommandOverride{
				CommandId:       "spacewave.palette",
				ReplaceBindings: true,
				Bindings: []*s4wave_command.CommandBinding{{
					Id: "palette-stale",
					Binding: &s4wave_command.CommandBinding_Combo{
						Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+P"},
					},
					When: s4wave_command.CommandFocusContext_COMMAND_FOCUS_CONTEXT_GLOBAL,
				}},
			},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	replacementUpsert, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertKeybindingOverride{
			UpsertKeybindingOverride: &s4wave_command.KeybindingCommandOverride{
				CommandId:         "spacewave.palette",
				ClearedBindingIds: []string{"palette-default"},
				Bindings: []*s4wave_command.CommandBinding{{
					Id: "palette-account",
					Binding: &s4wave_command.CommandBinding_Combo{
						Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"},
					},
					When: s4wave_command.CommandFocusContext_COMMAND_FOCUS_CONTEXT_GLOBAL,
				}},
			},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	missingCommandUpsert, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertKeybindingOverride{
			UpsertKeybindingOverride: &s4wave_command.KeybindingCommandOverride{
				Bindings: []*s4wave_command.CommandBinding{{
					Id: "missing-command",
					Binding: &s4wave_command.CommandBinding_Combo{
						Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+M"},
					},
					When: s4wave_command.CommandFocusContext_COMMAND_FOCUS_CONTEXT_GLOBAL,
				}},
			},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	missingCommandRemove, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveKeybindingOverride{
			RemoveKeybindingOverride: &account_settings.RemoveKeybindingOverrideOp{},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	nextData, results, err := account_settings.ProcessAccountSettingsOps(
		ctx,
		nil,
		currentData,
		[]*sobject.SOOperationInner{
			{PeerId: peerID, Nonce: 1, OpData: staleUpsert},
			{PeerId: peerID, Nonce: 2, OpData: replacementUpsert},
			{PeerId: peerID, Nonce: 3, OpData: missingCommandUpsert},
			{PeerId: peerID, Nonce: 4, OpData: missingCommandRemove},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if nextData == nil {
		t.Fatal("expected keybinding upserts to change account settings")
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 op results, got %d", len(results))
	}
	if !results[0].GetSuccess() || !results[1].GetSuccess() {
		t.Fatalf("expected both valid upserts to succeed: %#v", results[:2])
	}
	if results[2].GetSuccess() || results[2].GetErrorDetails().GetErrorMsg() != "command_id is required" {
		t.Fatalf("expected empty upsert command_id rejection, got %#v", results[2].GetErrorDetails())
	}
	if results[3].GetSuccess() || results[3].GetErrorDetails().GetErrorMsg() != "command_id is required" {
		t.Fatalf("expected empty remove command_id rejection, got %#v", results[3].GetErrorDetails())
	}

	next := &account_settings.AccountSettings{}
	if err := next.UnmarshalVT(*nextData); err != nil {
		t.Fatal(err)
	}
	overrides := next.GetKeybindingOverrides().GetOverrides()
	if len(overrides) != 2 {
		t.Fatalf("expected existing plus deduped palette override, got %d", len(overrides))
	}
	if overrides[0].GetCommandId() != "spacewave.existing" {
		t.Fatalf("existing override moved or dropped: %#v", overrides[0])
	}
	palette := overrides[1]
	if palette.GetCommandId() != "spacewave.palette" {
		t.Fatalf("expected palette override, got %q", palette.GetCommandId())
	}
	if palette.GetReplaceBindings() {
		t.Fatal("stale duplicate replace flag survived replacement upsert")
	}
	if got := palette.GetClearedBindingIds(); len(got) != 1 || got[0] != "palette-default" {
		t.Fatalf("replacement cleared ids = %#v", got)
	}
	if bindings := palette.GetBindings(); len(bindings) != 1 || bindings[0].GetId() != "palette-account" {
		t.Fatalf("replacement bindings = %#v", bindings)
	}

	unchangedData, malformedResults, err := account_settings.ProcessAccountSettingsOps(
		ctx,
		nil,
		*nextData,
		[]*sobject.SOOperationInner{{PeerId: peerID, Nonce: 5, OpData: []byte{0xff}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if unchangedData != nil {
		t.Fatal("malformed op changed account settings state")
	}
	if len(malformedResults) != 1 || malformedResults[0].GetSuccess() {
		t.Fatalf("expected malformed op rejection, got %#v", malformedResults)
	}

	removePalette, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveKeybindingOverride{
			RemoveKeybindingOverride: &account_settings.RemoveKeybindingOverrideOp{
				CommandId: "spacewave.palette",
			},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	removedData, removeResults, err := account_settings.ProcessAccountSettingsOps(
		ctx,
		nil,
		*nextData,
		[]*sobject.SOOperationInner{{PeerId: peerID, Nonce: 6, OpData: removePalette}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if removedData == nil {
		t.Fatal("expected remove to change account settings")
	}
	if len(removeResults) != 1 || !removeResults[0].GetSuccess() {
		t.Fatalf("expected valid remove to succeed, got %#v", removeResults)
	}
	removed := &account_settings.AccountSettings{}
	if err := removed.UnmarshalVT(*removedData); err != nil {
		t.Fatal(err)
	}
	remaining := removed.GetKeybindingOverrides().GetOverrides()
	if len(remaining) != 1 || remaining[0].GetCommandId() != "spacewave.existing" {
		t.Fatalf("remove should leave only the existing override, got %#v", remaining)
	}
}

func TestSetKeybindingSettingsPreservesCommandOverrides(t *testing.T) {
	ctx := t.Context()
	peerID := "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"
	initial := &account_settings.AccountSettings{
		KeybindingOverrides: &s4wave_command.KeybindingOverrideSet{
			Version: 1,
			Overrides: []*s4wave_command.KeybindingCommandOverride{{
				CommandId:       "spacewave.palette",
				ReplaceBindings: true,
				Bindings: []*s4wave_command.CommandBinding{{
					Id: "palette-account",
					Binding: &s4wave_command.CommandBinding_Combo{
						Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"},
					},
					When: s4wave_command.CommandFocusContext_COMMAND_FOCUS_CONTEXT_GLOBAL,
				}},
			}},
			Settings: &s4wave_command.KeybindingOverrideSettings{
				LeaderCombo:     "Ctrl+Space",
				WhichKeyDelayMs: 25,
			},
		},
	}
	currentData, err := initial.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	settingsOp, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_SetKeybindingSettings{
			SetKeybindingSettings: &s4wave_command.KeybindingOverrideSettings{
				LeaderCombo:     "Alt+Space",
				WhichKeyDelayMs: 175,
			},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	nextData, results, err := account_settings.ProcessAccountSettingsOps(
		ctx,
		nil,
		currentData,
		[]*sobject.SOOperationInner{{PeerId: peerID, Nonce: 1, OpData: settingsOp}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if nextData == nil {
		t.Fatal("expected settings write to change account settings")
	}
	if len(results) != 1 || !results[0].GetSuccess() {
		t.Fatalf("expected settings write to succeed, got %#v", results)
	}

	next := &account_settings.AccountSettings{}
	if err := next.UnmarshalVT(*nextData); err != nil {
		t.Fatal(err)
	}
	overrideSet := next.GetKeybindingOverrides()
	settings := overrideSet.GetSettings()
	if settings.GetLeaderCombo() != "Alt+Space" {
		t.Fatalf("leader combo = %q", settings.GetLeaderCombo())
	}
	if settings.GetWhichKeyDelayMs() != 175 {
		t.Fatalf("which-key delay = %d", settings.GetWhichKeyDelayMs())
	}
	overrides := overrideSet.GetOverrides()
	if len(overrides) != 1 {
		t.Fatalf("expected existing command override to remain, got %#v", overrides)
	}
	palette := overrides[0]
	if palette.GetCommandId() != "spacewave.palette" {
		t.Fatalf("command override command_id = %q", palette.GetCommandId())
	}
	if !palette.GetReplaceBindings() {
		t.Fatal("command override replace flag was dropped")
	}
	if bindings := palette.GetBindings(); len(bindings) != 1 || bindings[0].GetId() != "palette-account" || bindings[0].GetCombo().GetCombo() != "Ctrl+K" {
		t.Fatalf("command override bindings changed: %#v", bindings)
	}
}

func TestReplaceKeybindingOverrideSetRejectsConflictingPartition(t *testing.T) {
	ctx := t.Context()
	initialSet := &s4wave_command.KeybindingOverrideSet{
		Version: 2,
		WebOverrides: []*s4wave_command.KeybindingCommandOverride{{
			CommandId: "spacewave.palette",
		}},
	}
	initialData, err := (&account_settings.AccountSettings{
		KeybindingOverrides: initialSet,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	first := initialSet.CloneVT()
	first.WebSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+A"}
	second := initialSet.CloneVT()
	second.WebSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+B"}
	marshalOp := func(replacement *s4wave_command.KeybindingOverrideSet) []byte {
		t.Helper()
		data, err := (&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_ReplaceKeybindingOverrideSet{
				ReplaceKeybindingOverrideSet: &account_settings.ReplaceKeybindingOverrideSetOp{
					ExpectedOverrideSet: initialSet,
					OverrideSet:         replacement,
				},
			},
		}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	nextData, results, err := account_settings.ProcessAccountSettingsOps(
		ctx,
		nil,
		initialData,
		[]*sobject.SOOperationInner{
			{PeerId: "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW", Nonce: 1, OpData: marshalOp(first)},
			{PeerId: "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW", Nonce: 2, OpData: marshalOp(second)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || !results[0].GetSuccess() || results[1].GetSuccess() {
		t.Fatalf("expected one accepted replacement and one conflict, got %#v", results)
	}
	got := &account_settings.AccountSettings{}
	if nextData == nil {
		t.Fatal("expected changed account settings")
	}
	if err := got.UnmarshalVT(*nextData); err != nil {
		t.Fatal(err)
	}
	if !got.GetKeybindingOverrides().EqualVT(first) {
		t.Fatalf("conflicting replacement changed winner: %#v", got.GetKeybindingOverrides())
	}
}

func TestHistoricalKeybindingOpsUpdateVersionTwoWebPartition(t *testing.T) {
	ctx := t.Context()
	initial := &account_settings.AccountSettings{
		KeybindingOverrides: &s4wave_command.KeybindingOverrideSet{
			Version: 2,
			WebOverrides: []*s4wave_command.KeybindingCommandOverride{{
				CommandId: "remove-me",
			}},
			TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{
				CommandId: "tui-only",
			}},
			TuiSettings: &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+T"},
		},
	}
	initialData, err := initial.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	marshalOp := func(op *account_settings.AccountSettingsOp) []byte {
		t.Helper()
		data, err := op.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	peerID := "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"
	nextData, results, err := account_settings.ProcessAccountSettingsOps(
		ctx,
		nil,
		initialData,
		[]*sobject.SOOperationInner{
			{PeerId: peerID, Nonce: 1, OpData: marshalOp(&account_settings.AccountSettingsOp{
				Op: &account_settings.AccountSettingsOp_UpsertKeybindingOverride{
					UpsertKeybindingOverride: &s4wave_command.KeybindingCommandOverride{
						CommandId: "late-web",
						Bindings: []*s4wave_command.CommandBinding{{
							Id:      "legacy",
							Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+L"}},
						}},
					},
				},
			})},
			{PeerId: peerID, Nonce: 2, OpData: marshalOp(&account_settings.AccountSettingsOp{
				Op: &account_settings.AccountSettingsOp_RemoveKeybindingOverride{
					RemoveKeybindingOverride: &account_settings.RemoveKeybindingOverrideOp{CommandId: "remove-me"},
				},
			})},
			{PeerId: peerID, Nonce: 3, OpData: marshalOp(&account_settings.AccountSettingsOp{
				Op: &account_settings.AccountSettingsOp_SetKeybindingSettings{
					SetKeybindingSettings: &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+W"},
				},
			})},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || !results[0].GetSuccess() || !results[1].GetSuccess() || !results[2].GetSuccess() {
		t.Fatalf("historical v1 operations rejected: %#v", results)
	}
	got := &account_settings.AccountSettings{}
	if nextData == nil {
		t.Fatal("expected changed account settings")
	}
	if err := got.UnmarshalVT(*nextData); err != nil {
		t.Fatal(err)
	}
	overrideSet := got.GetKeybindingOverrides()
	if overrideSet.GetVersion() != 2 || len(overrideSet.GetOverrides()) != 0 || overrideSet.GetSettings() != nil {
		t.Fatalf("created hybrid v2 state: %#v", overrideSet)
	}
	if len(overrideSet.GetWebOverrides()) != 1 || overrideSet.GetWebOverrides()[0].GetCommandId() != "late-web" ||
		overrideSet.GetWebOverrides()[0].GetBindings()[0].GetSurface() != s4wave_command.CommandSurface_COMMAND_SURFACE_WEB {
		t.Fatalf("late upsert did not enter WEB partition: %#v", overrideSet.GetWebOverrides())
	}
	if overrideSet.GetWebSettings().GetLeaderCombo() != "Ctrl+W" ||
		overrideSet.GetTuiSettings().GetLeaderCombo() != "Ctrl+T" ||
		len(overrideSet.GetTuiOverrides()) != 1 {
		t.Fatalf("late v1 operation changed TUI or settings: %#v", overrideSet)
	}
}
