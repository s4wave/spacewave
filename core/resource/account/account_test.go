package resource_account_test

import (
	"context"
	"crypto/rand"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	"github.com/s4wave/spacewave/testbed"
)

// TestWatchAccountInfoLocal verifies the account info watch for a local account.
func TestWatchAccountInfoLocal(t *testing.T) {
	ctx := t.Context()

	tb, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	displayNameOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpdateDisplayName{
			UpdateDisplayName: &account_settings.UpdateDisplayNameOp{
				DisplayName: "Local Workstation",
			},
		},
	}
	displayNameData, err := displayNameOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, displayNameData)

	keypairOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     "12D3KooWLocalKeypair",
				AuthMethod: "password",
			},
		},
	}
	keypairData, err := keypairOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, keypairData)

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	var received *s4wave_account.WatchAccountInfoResponse
	strm := &testWatchAccountInfoStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_account.WatchAccountInfoResponse) error {
			if resp.GetEntityId() == "Local Workstation" && resp.GetKeypairCount() == 1 {
				received = resp
				rpcCancel()
			}
			return nil
		},
	}

	err = ar.WatchAccountInfo(&s4wave_account.WatchAccountInfoRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	if received == nil {
		t.Fatal("expected local account info snapshot")
	}
	if received.GetAccountId() != accountID {
		t.Fatalf("expected account id %q, got %q", accountID, received.GetAccountId())
	}
	if received.GetProviderId() != "local" {
		t.Fatalf("expected provider id local, got %q", received.GetProviderId())
	}
	if received.GetEntityId() != "Local Workstation" {
		t.Fatalf("expected entity id %q, got %q", "Local Workstation", received.GetEntityId())
	}
	if received.GetKeypairCount() != 1 {
		t.Fatalf("expected keypair count 1, got %d", received.GetKeypairCount())
	}
}

func TestWatchEntityKeypairsLocalStreamsAccountSettingsKeypairs(t *testing.T) {
	ctx := t.Context()

	tb, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	peerID := generateTestPeerID(t)
	keypairOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     peerID,
				AuthMethod: auth_password.MethodID,
			},
		},
	}
	keypairData, err := keypairOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, keypairData)
	waitForAccountSettings(ctx, t, so, func(settings *account_settings.AccountSettings) bool {
		return hasEntityKeypair(settings, peerID, auth_password.MethodID)
	})

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	var received *s4wave_account.WatchEntityKeypairsResponse
	strm := &testWatchEntityKeypairsStream{
		ctx: ctx,
		onSend: func(resp *s4wave_account.WatchEntityKeypairsResponse) error {
			received = resp
			for _, state := range resp.GetKeypairs() {
				keypair := state.GetKeypair()
				if keypair.GetPeerId() == peerID && keypair.GetAuthMethod() == auth_password.MethodID {
					return io.EOF
				}
			}
			return nil
		},
	}

	err = ar.WatchEntityKeypairs(&s4wave_account.WatchEntityKeypairsRequest{}, strm)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if received == nil {
		t.Fatal("expected local entity keypair snapshot")
	}
	if received.GetUnlockedCount() != 0 {
		t.Fatalf("expected no unlocked local keypairs, got %d", received.GetUnlockedCount())
	}
	keypairs := received.GetKeypairs()
	if len(keypairs) != 1 {
		t.Fatalf("expected 1 entity keypair, got %d", len(keypairs))
	}
	state := keypairs[0]
	if state.GetUnlocked() {
		t.Fatal("expected local account keypair stream to report locked keypairs")
	}
	keypair := state.GetKeypair()
	if keypair.GetPeerId() != peerID {
		t.Fatalf("expected peer id %q, got %q", peerID, keypair.GetPeerId())
	}
	if keypair.GetAuthMethod() != auth_password.MethodID {
		t.Fatalf("expected auth method %q, got %q", auth_password.MethodID, keypair.GetAuthMethod())
	}
}

func TestGenerateBackupKeyLocalPersistsPEMKeypair(t *testing.T) {
	ctx := t.Context()

	tb, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	resp, err := ar.GenerateBackupKey(ctx, &s4wave_account.GenerateBackupKeyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetPemData()) == 0 {
		t.Fatal("expected backup key PEM data")
	}
	if resp.GetPeerId() == "" {
		t.Fatal("expected backup key peer id")
	}
	backupPriv, err := keypem.ParsePrivKeyPem(resp.GetPemData())
	if err != nil {
		t.Fatalf("parse generated backup PEM: %v", err)
	}
	backupPeerID, err := peer.IDFromPrivateKey(backupPriv)
	if err != nil {
		t.Fatalf("derive generated backup peer id: %v", err)
	}
	if backupPeerID.String() != resp.GetPeerId() {
		t.Fatalf("PEM peer id = %q, response peer id = %q", backupPeerID.String(), resp.GetPeerId())
	}

	settings := waitForAccountSettings(ctx, t, so, func(settings *account_settings.AccountSettings) bool {
		return hasEntityKeypair(settings, resp.GetPeerId(), "pem")
	})
	var matched *session.EntityKeypair
	for _, kp := range settings.GetEntityKeypairs() {
		if kp.GetPeerId() == resp.GetPeerId() {
			matched = kp
			break
		}
	}
	if matched == nil {
		t.Fatalf("expected AccountSettings PEM keypair row for %q", resp.GetPeerId())
	}
	if matched.GetAuthMethod() != "pem" {
		t.Fatalf("expected persisted backup auth method %q, got %q", "pem", matched.GetAuthMethod())
	}
}

func TestGenerateBackupKeyLocalDoesNotAddCredentialKeypair(t *testing.T) {
	ctx := t.Context()

	tb, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	resp, err := ar.GenerateBackupKey(ctx, &s4wave_account.GenerateBackupKeyRequest{
		Credential: &session.EntityCredential{
			Credential: &session.EntityCredential_Password{
				Password: "independent-backup-key",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	settings := waitForAccountSettings(ctx, t, so, func(settings *account_settings.AccountSettings) bool {
		return hasEntityKeypair(settings, resp.GetPeerId(), "pem")
	})
	keypairs := settings.GetEntityKeypairs()
	if len(keypairs) != 1 {
		t.Fatalf("expected only the backup keypair, got %d keypairs", len(keypairs))
	}
	if keypairs[0].GetPeerId() != resp.GetPeerId() {
		t.Fatalf("expected backup peer id %q, got %q", resp.GetPeerId(), keypairs[0].GetPeerId())
	}
	if keypairs[0].GetAuthMethod() != "pem" {
		t.Fatalf("expected backup auth method %q, got %q", "pem", keypairs[0].GetAuthMethod())
	}
}

func TestResolveEntityKeyLocalPasswordUsesAccountID(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("production-cost password scrypt is too slow under GoScript")
	}

	ctx := t.Context()

	_, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	password := "local-resolve-password"
	_, expectedPriv, err := auth_password.BuildParametersWithUsernamePassword(accountID, []byte(password))
	if err != nil {
		t.Fatal(err)
	}
	expectedPeerID, err := peer.IDFromPrivateKey(expectedPriv)
	if err != nil {
		t.Fatal(err)
	}

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	priv, gotPeerID, err := ar.ResolveEntityKey(ctx, &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: password},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPeerID != expectedPeerID {
		t.Fatalf("expected password peer id %q derived from local account id %q, got %q", expectedPeerID, accountID, gotPeerID)
	}

	privPeerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if privPeerID != expectedPeerID {
		t.Fatalf("resolved private key peer id = %q, expected %q", privPeerID, expectedPeerID)
	}
}

func TestChangePasswordLocalReplacesPasswordKeypair(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("production-cost password scrypt is too slow under GoScript; PEM credential resource coverage runs separately")
	}

	ctx := t.Context()

	tb, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	oldPassword := "old-local-password"
	newPassword := "new-local-password"
	oldPeerID := derivePasswordPeerID(t, accountID, oldPassword)
	newPeerID := derivePasswordPeerID(t, accountID, newPassword)

	oldKeypairOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     oldPeerID,
				AuthMethod: auth_password.MethodID,
			},
		},
	}
	oldKeypairData, err := oldKeypairOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, oldKeypairData)
	waitForAccountSettings(ctx, t, so, func(settings *account_settings.AccountSettings) bool {
		return hasEntityKeypair(settings, oldPeerID, auth_password.MethodID)
	})

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	if _, err := ar.ChangePassword(ctx, &s4wave_account.ChangePasswordRequest{
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}); err != nil {
		t.Fatal(err)
	}

	settings := waitForAccountSettings(ctx, t, so, func(settings *account_settings.AccountSettings) bool {
		keypairs := settings.GetEntityKeypairs()
		return len(keypairs) == 1 &&
			keypairs[0].GetPeerId() == newPeerID &&
			keypairs[0].GetAuthMethod() == auth_password.MethodID
	})
	keypairs := settings.GetEntityKeypairs()
	if len(keypairs) != 1 {
		t.Fatalf("expected exactly one password keypair after password change, got %d", len(keypairs))
	}
	keypair := keypairs[0]
	if keypair.GetPeerId() != newPeerID {
		t.Fatalf("expected new password peer id %q, got %q", newPeerID, keypair.GetPeerId())
	}
	if keypair.GetPeerId() == oldPeerID {
		t.Fatalf("old password peer id %q remained after password change", oldPeerID)
	}
	if keypair.GetAuthMethod() != auth_password.MethodID {
		t.Fatalf("expected auth method %q, got %q", auth_password.MethodID, keypair.GetAuthMethod())
	}
}

// TestWatchSessionsLocal verifies the account sessions watch for a local account.
func TestWatchSessionsLocal(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()
	sess, sessRelease, err := session.ExMountSession(
		ctx,
		tb.Bus,
		sessRef,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      "12D3KooWRemotePeer1",
				DisplayName: "Remote Device",
				PairedAt:    1000,
			},
		},
	}
	addOpData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, addOpData)

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	currentPeerID := sess.GetPeerId().String()
	if currentPeerID == "" {
		t.Fatal("expected mounted session peer ID")
	}
	currentPresOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     currentPeerID,
				Label:      "Workstation",
				DeviceType: "desktop",
				ClientName: "Alpha desktop",
				Location:   "Portland, OR",
			},
		},
	}
	currentPresData, err := currentPresOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, currentPresData)
	remotePresOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     "12D3KooWRemotePeer1",
				ClientName: "Linked device",
				Location:   "Home Office",
			},
		},
	}
	remotePresData, err := remotePresOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, remotePresData)

	var received *s4wave_account.WatchSessionsResponse
	strm := &testWatchSessionsStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_account.WatchSessionsResponse) error {
			if len(resp.GetSessions()) >= 2 {
				current := resp.GetSessions()[0]
				remote := resp.GetSessions()[1]
				if current.GetLabel() == "Workstation" &&
					current.GetClientName() == "Alpha desktop" &&
					remote.GetClientName() == "Linked device" {
					received = resp
					rpcCancel()
				}
			}
			return nil
		},
	}

	err = ar.WatchSessions(&s4wave_account.WatchSessionsRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	if received == nil {
		t.Fatal("expected local sessions snapshot")
	}
	if len(received.GetSessions()) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(received.GetSessions()))
	}

	current := received.GetSessions()[0]
	if current.GetPeerId() != currentPeerID {
		t.Fatalf("expected current peer_id %q, got %q", currentPeerID, current.GetPeerId())
	}
	if !current.GetCurrentSession() {
		t.Fatal("expected current session row to be marked current")
	}
	if current.GetKind() != s4wave_account.AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION {
		t.Fatalf("expected local session kind, got %v", current.GetKind())
	}
	if current.GetLabel() != "Workstation" {
		t.Fatalf("expected current label %q, got %q", "Workstation", current.GetLabel())
	}
	if current.GetClientName() != "Alpha desktop" {
		t.Fatalf("expected current client name %q, got %q", "Alpha desktop", current.GetClientName())
	}
	if current.GetLocation() != "Portland, OR" {
		t.Fatalf("expected current location %q, got %q", "Portland, OR", current.GetLocation())
	}

	remote := received.GetSessions()[1]
	if remote.GetPeerId() != "12D3KooWRemotePeer1" {
		t.Fatalf("expected remote peer_id %q, got %q", "12D3KooWRemotePeer1", remote.GetPeerId())
	}
	if remote.GetCurrentSession() {
		t.Fatal("expected remote session row to be non-current")
	}
	if remote.GetKind() != s4wave_account.AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION {
		t.Fatalf("expected local session kind, got %v", remote.GetKind())
	}
	if remote.GetClientName() != "Linked device" {
		t.Fatalf("expected remote client name %q, got %q", "Linked device", remote.GetClientName())
	}
	if remote.GetLocation() != "Home Office" {
		t.Fatalf("expected remote location %q, got %q", "Home Office", remote.GetLocation())
	}
}

func TestRevokeSessionLocalReturnsUnsupported(t *testing.T) {
	ctx := t.Context()

	_, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	_, err := ar.RevokeSession(ctx, &s4wave_account.RevokeSessionRequest{
		SessionPeerId: "12D3KooWRemotePeer1",
	})
	if err == nil {
		t.Fatal("expected unsupported revoke error")
	}
	if !strings.Contains(err.Error(), "cloud account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetKeybindingSettingsHistoricalOpReplayPreservesOverrides(t *testing.T) {
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
	if !palette.GetReplaceBindings() {
		t.Fatal("command override replace flag was dropped")
	}
	if bindings := palette.GetBindings(); len(bindings) != 1 || bindings[0].GetId() != "palette-account" || bindings[0].GetCombo().GetCombo() != "Ctrl+K" {
		t.Fatalf("command override bindings changed: %#v", bindings)
	}
}

func TestReplaceKeybindingOverrideSetAtomicValidation(t *testing.T) {
	ctx := t.Context()
	_, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()
	ar := resource_account.NewAccountResource(acc)
	valid := &s4wave_command.KeybindingOverrideSet{
		Version:      2,
		WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "spacewave.palette", Bindings: []*s4wave_command.CommandBinding{{Id: "palette-web", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB}}}},
		TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "spacewave.palette", Bindings: []*s4wave_command.CommandBinding{{Id: "palette-tui", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI}}}},
	}
	expected := &s4wave_command.KeybindingOverrideSet{Version: 1}
	for i := range 2 {
		if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{
			ExpectedOverrideSet: expected,
			OverrideSet:         valid,
		}); err != nil {
			t.Fatalf("valid replacement %d: %v", i, err)
		}
		expected = valid
	}

	concurrent := valid.CloneVT()
	concurrent.WebSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+Space"}
	if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{
		ExpectedOverrideSet: valid,
		OverrideSet:         concurrent,
	}); err != nil {
		t.Fatalf("concurrent winner: %v", err)
	}
	concurrentTUI := valid.CloneVT()
	concurrentTUI.TuiSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+B"}
	if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{
		ExpectedOverrideSet: valid,
		OverrideSet:         concurrentTUI,
	}); err != nil {
		t.Fatalf("concurrent TUI replacement: %v", err)
	}
	merged := concurrent.CloneVT()
	merged.TuiSettings = concurrentTUI.TuiSettings.CloneVT()
	invalid := []*s4wave_command.KeybindingOverrideSet{
		{Version: 1},
		{Version: 2, Overrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "legacy"}}},
		{Version: 2, WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "dup"}, {CommandId: "dup"}}},
		{Version: 2, TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "dup"}, {CommandId: "dup"}}},
		{Version: 2, WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "tui-in-web", Bindings: []*s4wave_command.CommandBinding{{Id: "tui-in-web", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "x"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI}}}}},
		{Version: 2, TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "web-in-tui", Bindings: []*s4wave_command.CommandBinding{{Id: "web-in-tui", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "x"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB}}}}},
		{Version: 2, WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "unknown", Bindings: []*s4wave_command.CommandBinding{{Id: "unknown", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "x"}}}}}}},
	}
	for i, value := range invalid {
		if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{ExpectedOverrideSet: merged, OverrideSet: value}); err == nil {
			t.Fatalf("invalid replacement %d accepted", i)
		}
	}
	rpcCtx, cancel := context.WithCancel(ctx)
	var got *s4wave_command.KeybindingOverrideSet
	strm := &testWatchKeybindingOverridesStream{ctx: rpcCtx, onSend: func(resp *s4wave_account.WatchKeybindingOverridesResponse) error {
		if resp.GetOverrideSet().EqualVT(merged) {
			got = resp.GetOverrideSet()
			cancel()
		}
		return nil
	}}
	if err := ar.WatchKeybindingOverrides(&s4wave_account.WatchKeybindingOverridesRequest{}, strm); err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}
	if !got.EqualVT(merged) {
		t.Fatalf("rejected operation changed snapshot: %#v", got)
	}
}

func setupLocalProviderAccount(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, *session.SessionRef, string, *provider_local.ProviderAccount, func()) {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     peerID.String(),
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

func mountLocalAccountSettingsSO(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	acc *provider_local.ProviderAccount,
) (sobject.SharedObject, func()) {
	t.Helper()

	ref, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	so, mountRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return so, func() { mountRef.Release() }
}

func waitForAccountSettings(
	ctx context.Context,
	t *testing.T,
	so sobject.SharedObject,
	valid func(*account_settings.AccountSettings) bool,
) *account_settings.AccountSettings {
	t.Helper()

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
			settings = decodeAccountSettings(ctx, t, snap)
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
	if settings == nil {
		t.Fatal("expected account settings state")
	}
	return settings
}

func hasEntityKeypair(settings *account_settings.AccountSettings, peerID string, authMethod string) bool {
	for _, kp := range settings.GetEntityKeypairs() {
		if kp.GetPeerId() == peerID && kp.GetAuthMethod() == authMethod {
			return true
		}
	}
	return false
}

func queueAccountSettingsOp(
	ctx context.Context,
	t *testing.T,
	so sobject.SharedObject,
	opData []byte,
) {
	t.Helper()

	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		t.Fatal(err)
	}
	if _, wasRejected, err := so.WaitOperation(ctx, localID); err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		t.Fatal(err)
	}
}

func decodeAccountSettings(
	ctx context.Context,
	t *testing.T,
	snap sobject.SharedObjectStateSnapshot,
) *account_settings.AccountSettings {
	t.Helper()

	settings := &account_settings.AccountSettings{}
	if snap == nil {
		return settings
	}
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rootInner == nil || len(rootInner.GetStateData()) == 0 {
		return settings
	}
	if err := settings.UnmarshalVT(rootInner.GetStateData()); err != nil {
		t.Fatal(err)
	}
	return settings
}

func generateTestPeerID(t *testing.T) string {
	t.Helper()

	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid.String()
}

func derivePasswordPeerID(t *testing.T, accountID string, password string) string {
	t.Helper()

	_, priv, err := auth_password.BuildParametersWithUsernamePassword(accountID, []byte(password))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid.String()
}

type testWatchSessionsStream struct {
	ctx    context.Context
	onSend func(*s4wave_account.WatchSessionsResponse) error
}

func (s *testWatchSessionsStream) Context() context.Context     { return s.ctx }
func (s *testWatchSessionsStream) MsgRecv(_ srpc.Message) error { return nil }
func (s *testWatchSessionsStream) CloseSend() error             { return nil }
func (s *testWatchSessionsStream) Close() error                 { return nil }
func (s *testWatchSessionsStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchSessionsStream) Send(resp *s4wave_account.WatchSessionsResponse) error {
	return s.onSend(resp)
}

func (s *testWatchSessionsStream) SendAndClose(resp *s4wave_account.WatchSessionsResponse) error {
	return s.onSend(resp)
}

type testWatchKeybindingOverridesStream struct {
	ctx    context.Context
	onSend func(*s4wave_account.WatchKeybindingOverridesResponse) error
}

func (s *testWatchKeybindingOverridesStream) Context() context.Context { return s.ctx }
func (s *testWatchKeybindingOverridesStream) MsgRecv(_ srpc.Message) error {
	return nil
}
func (s *testWatchKeybindingOverridesStream) CloseSend() error             { return nil }
func (s *testWatchKeybindingOverridesStream) Close() error                 { return nil }
func (s *testWatchKeybindingOverridesStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchKeybindingOverridesStream) Send(resp *s4wave_account.WatchKeybindingOverridesResponse) error {
	return s.onSend(resp)
}

func (s *testWatchKeybindingOverridesStream) SendAndClose(resp *s4wave_account.WatchKeybindingOverridesResponse) error {
	return s.onSend(resp)
}

type testWatchAccountInfoStream struct {
	ctx    context.Context
	onSend func(*s4wave_account.WatchAccountInfoResponse) error
}

func (s *testWatchAccountInfoStream) Context() context.Context     { return s.ctx }
func (s *testWatchAccountInfoStream) MsgRecv(_ srpc.Message) error { return nil }
func (s *testWatchAccountInfoStream) CloseSend() error             { return nil }
func (s *testWatchAccountInfoStream) Close() error                 { return nil }
func (s *testWatchAccountInfoStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchAccountInfoStream) Send(resp *s4wave_account.WatchAccountInfoResponse) error {
	return s.onSend(resp)
}

func (s *testWatchAccountInfoStream) SendAndClose(resp *s4wave_account.WatchAccountInfoResponse) error {
	return s.onSend(resp)
}

type testWatchEntityKeypairsStream struct {
	ctx    context.Context
	onSend func(*s4wave_account.WatchEntityKeypairsResponse) error
}

func (s *testWatchEntityKeypairsStream) Context() context.Context { return s.ctx }
func (s *testWatchEntityKeypairsStream) MsgRecv(_ srpc.Message) error {
	return nil
}
func (s *testWatchEntityKeypairsStream) CloseSend() error             { return nil }
func (s *testWatchEntityKeypairsStream) Close() error                 { return nil }
func (s *testWatchEntityKeypairsStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchEntityKeypairsStream) Send(resp *s4wave_account.WatchEntityKeypairsResponse) error {
	return s.onSend(resp)
}

func (s *testWatchEntityKeypairsStream) SendAndClose(resp *s4wave_account.WatchEntityKeypairsResponse) error {
	return s.onSend(resp)
}
