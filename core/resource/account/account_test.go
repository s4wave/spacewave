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
	// Use a test-scoped context for the local account watch.
	ctx := t.Context()

	// Start the local provider account and retain its cleanup.
	tb, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Mount the account settings SharedObject used by this watch.
	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	// Prepare the display-name operation for the account snapshot.
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

	// Prepare the keypair operation included in the account snapshot.
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

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Cancel the RPC stream after the expected account snapshot arrives.
	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	// Capture the streamed account response and cancel after matching settings.
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

	// Run the account-info watch until the test stream cancels it.
	err = ar.WatchAccountInfo(&s4wave_account.WatchAccountInfoRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	// Assert the response contains the local account identity and keypair count.
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
	// Use a test-scoped context for the local keypair watch.
	ctx := t.Context()

	// Start the local provider account and retain its cleanup.
	tb, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Mount the account settings SharedObject for the keypair stream.
	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	// Create and persist the test entity keypair before opening the watch.
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

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Capture keypair updates until the expected peer and method arrive.
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

	// Run the local keypair watch through the test stream.
	err = ar.WatchEntityKeypairs(&s4wave_account.WatchEntityKeypairsRequest{}, strm)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Assert the watch reports the persisted keypair as locked.
	if received == nil {
		t.Fatal("expected local entity keypair snapshot")
	}
	if received.GetUnlockedCount() != 0 {
		t.Fatalf("expected no unlocked local keypairs, got %d", received.GetUnlockedCount())
	}

	// Check the keypair count, lock state, peer ID, and authentication method.
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
	// Use a test-scoped context for backup-key generation.
	ctx := t.Context()

	// Start the local provider account and retain its cleanup.
	tb, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Mount account settings to verify backup-key persistence.
	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Generate a local backup key and verify its returned PEM identity.
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

	// Verify the returned PEM decodes to the generated backup peer ID.
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

	// Wait for the backup keypair to appear in account settings.
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
	// Use a test-scoped context for the credential-keypair isolation check.
	ctx := t.Context()

	// Start the local provider account and retain its cleanup.
	tb, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Mount account settings to inspect the persisted keypairs.
	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Generate a backup key using a separate password credential.
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

	// Wait until settings contain only the generated PEM backup keypair.
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
	// Skip the expensive password derivation only under GoScript.
	if runtime.GOOS == "js" {
		t.Skip("production-cost password scrypt is too slow under GoScript")
	}

	// Use a test-scoped context for local entity-key resolution.
	ctx := t.Context()

	// Create the local account whose ID is the password derivation input.
	_, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Derive the expected private key and peer ID from the account ID.
	password := "local-resolve-password"
	_, expectedPriv, err := auth_password.BuildParametersWithUsernamePassword(accountID, []byte(password))
	if err != nil {
		t.Fatal(err)
	}
	expectedPeerID, err := peer.IDFromPrivateKey(expectedPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Resolve the password credential through the local AccountResource.
	priv, gotPeerID, err := ar.ResolveEntityKey(ctx, &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: password},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPeerID != expectedPeerID {
		t.Fatalf("expected password peer id %q derived from local account id %q, got %q", expectedPeerID, accountID, gotPeerID)
	}

	// Verify the resolved private key has the expected peer identity.
	privPeerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if privPeerID != expectedPeerID {
		t.Fatalf("resolved private key peer id = %q, expected %q", privPeerID, expectedPeerID)
	}
}

func TestChangePasswordLocalReplacesPasswordKeypair(t *testing.T) {
	// Skip password-derivation coverage only under GoScript.
	if runtime.GOOS == "js" {
		t.Skip("production-cost password scrypt is too slow under GoScript; PEM credential resource coverage runs separately")
	}

	// Use a test-scoped context for local password rotation.
	ctx := t.Context()

	// Start the local account and retain its identity and cleanup.
	tb, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Mount account settings to verify the keypair replacement.
	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	// Derive the old and new password identities for this account.
	oldPassword := "old-local-password"
	newPassword := "new-local-password"
	oldPeerID := derivePasswordPeerID(t, accountID, oldPassword)
	newPeerID := derivePasswordPeerID(t, accountID, newPassword)

	// Prepare the old password keypair in local account settings.
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

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Change the account password from the old credential to the new one.
	if _, err := ar.ChangePassword(ctx, &s4wave_account.ChangePasswordRequest{
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for settings to contain only the replacement password keypair.
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
	// Use a test-scoped context for the local sessions watch.
	ctx := t.Context()

	// Start the account and mount its current session for the watch.
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

	// Mount the account settings SharedObject for session metadata.
	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	// Add a paired device that the sessions watch must include.
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

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Cancel the RPC stream after the session rows are observed.
	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	// Read the mounted session identity before setting its presentation.
	currentPeerID := sess.GetPeerId().String()
	if currentPeerID == "" {
		t.Fatal("expected mounted session peer ID")
	}

	// Set a presentation for the current local session.
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

	// Set a presentation for the paired remote device.
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

	// Prepare a paired account member without a paired-device row.
	// Pairing enrolls an account member with a presentation and no paired device.
	agent, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	agentPeerID := agent.GetPeerID().String()

	// Persist the member and presentation operations in account settings.
	for _, op := range []*account_settings.AccountSettingsOp{
		{Op: &account_settings.AccountSettingsOp_UpsertAccountSession{
			UpsertAccountSession: &account_settings.AccountSession{PeerId: agentPeerID, StoragePeerId: agentPeerID},
		}},
		{Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{PeerId: agentPeerID, Label: "Agent on build host"},
		}},
	} {
		data, err := op.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		queueAccountSettingsOp(ctx, t, so, data)
	}

	// Capture the session snapshot after all three records are present.
	var received *s4wave_account.WatchSessionsResponse
	strm := &testWatchSessionsStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_account.WatchSessionsResponse) error {
			if len(resp.GetSessions()) >= 3 {
				current := resp.GetSessions()[0]
				remote := resp.GetSessions()[1]
				if current.GetLabel() == "Workstation" &&
					current.GetClientName() == "Alpha desktop" &&
					remote.GetClientName() == "Linked device" &&
					resp.GetSessions()[2].GetLabel() == "Agent on build host" {
					received = resp
					rpcCancel()
				}
			}
			return nil
		},
	}

	// Run the local sessions watch through the test stream.
	err = ar.WatchSessions(&s4wave_account.WatchSessionsRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	// Assert the watch emits all three expected session rows.
	if received == nil {
		t.Fatal("expected local sessions snapshot")
	}
	if len(received.GetSessions()) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(received.GetSessions()))
	}

	// Check the current-device row and its presentation fields.
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

	// Check the paired-device row and its presentation fields.
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

	// Check the account member that has no paired-device record.
	member := received.GetSessions()[2]
	if member.GetPeerId() != agentPeerID || member.GetCurrentSession() {
		t.Fatalf("expected paired member row for %q, got %v", agentPeerID, member)
	}
}

func TestRevokeSessionLocalReturnsUnsupported(t *testing.T) {
	// Use a test-scoped context for local session revocation.
	ctx := t.Context()

	// Start the local provider account and retain its cleanup.
	_, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	// Construct the AccountResource over the local provider account.
	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	// Request revocation of a remote session through the local account.
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

func TestReplaceKeybindingOverrideSetAtomicValidation(t *testing.T) {
	// Use a test-scoped context for atomic override validation.
	ctx := t.Context()

	// Create the local account and AccountResource under test.
	_, _, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()
	ar := resource_account.NewAccountResource(acc)

	// Define a valid override set for both Web and TUI surfaces.
	valid := &s4wave_command.KeybindingOverrideSet{
		WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "spacewave.palette", Bindings: []*s4wave_command.CommandBinding{{Id: "palette-web", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB}}}},
		TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "spacewave.palette", Bindings: []*s4wave_command.CommandBinding{{Id: "palette-tui", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI}}}},
	}

	// Apply the valid replacement twice with each prior value as its expectation.
	expected := &s4wave_command.KeybindingOverrideSet{}
	for i := range 2 {
		if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{
			ExpectedOverrideSet: expected,
			OverrideSet:         valid,
		}); err != nil {
			t.Fatalf("valid replacement %d: %v", i, err)
		}
		expected = valid
	}

	// Prepare independent concurrent updates to the same override set.
	concurrent := valid.CloneVT()
	concurrent.WebSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+Space"}

	// Apply the Web settings update against the shared expected state.
	if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{
		ExpectedOverrideSet: valid,
		OverrideSet:         concurrent,
	}); err != nil {
		t.Fatalf("concurrent winner: %v", err)
	}

	// Prepare a concurrent TUI settings update from the same expected state.
	concurrentTUI := valid.CloneVT()
	concurrentTUI.TuiSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+B"}

	// Apply the TUI settings update against the shared expected state.
	if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{
		ExpectedOverrideSet: valid,
		OverrideSet:         concurrentTUI,
	}); err != nil {
		t.Fatalf("concurrent TUI replacement: %v", err)
	}

	// Build the merged state that both successful updates should preserve.
	merged := concurrent.CloneVT()
	merged.TuiSettings = concurrentTUI.TuiSettings.CloneVT()

	// Define invalid override sets that must be rejected atomically.
	invalid := []*s4wave_command.KeybindingOverrideSet{
		{WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "dup"}, {CommandId: "dup"}}},
		{TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "dup"}, {CommandId: "dup"}}},
		{WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "tui-in-web", Bindings: []*s4wave_command.CommandBinding{{Id: "tui-in-web", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "x"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI}}}}},
		{TuiOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "web-in-tui", Bindings: []*s4wave_command.CommandBinding{{Id: "web-in-tui", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "x"}}, Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB}}}}},
		{WebOverrides: []*s4wave_command.KeybindingCommandOverride{{CommandId: "unknown", Bindings: []*s4wave_command.CommandBinding{{Id: "unknown", Binding: &s4wave_command.CommandBinding_Combo{Combo: &s4wave_command.KeyCombo{Combo: "x"}}}}}}},
	}

	// Reject every invalid replacement without changing the stored snapshot.
	for i, value := range invalid {
		if _, err := ar.ReplaceKeybindingOverrideSet(ctx, &s4wave_account.ReplaceKeybindingOverrideSetRequest{ExpectedOverrideSet: merged, OverrideSet: value}); err == nil {
			t.Fatalf("invalid replacement %d accepted", i)
		}
	}

	// Watch the final override state while the rejected operations are checked.
	rpcCtx, cancel := context.WithCancel(ctx)
	var got *s4wave_command.KeybindingOverrideSet
	strm := &testWatchKeybindingOverridesStream{ctx: rpcCtx, onSend: func(resp *s4wave_account.WatchKeybindingOverridesResponse) error {
		if resp.GetOverrideSet().EqualVT(merged) {
			got = resp.GetOverrideSet()
			cancel()
		}
		return nil
	}}

	// Read the final override snapshot through the production watch RPC.
	if err := ar.WatchKeybindingOverrides(&s4wave_account.WatchKeybindingOverridesRequest{}, strm); err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	// Assert rejected updates left the merged override set unchanged.
	if !got.EqualVT(merged) {
		t.Fatalf("rejected operation changed snapshot: %#v", got)
	}
}

func setupLocalProviderAccount(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, *session.SessionRef, string, *provider_local.ProviderAccount, func()) {
	// Run setup failures at the calling test site.
	t.Helper()

	// Start the in-memory testbed for the local account.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Register and start the local provider controller.
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

	// Resolve the local provider before creating its account and session.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	// Create a local account session through the provider.
	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	// Access the created provider account and construct its release chain.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	// Release the account, provider references, and testbed together.
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
	// Run mount failures at the calling test site.
	t.Helper()

	// Get the account settings reference and mount its SharedObject.
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
	// Run settings-watch failures at the calling test site.
	t.Helper()

	// Acquire account settings state for the watch assertion.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	// Retain the latest decoded settings while watching state changes.
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

	// Accept cancellation from the matching account settings state only.
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Require the watch to observe an account settings snapshot.
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
	// Run operation failures at the calling test site.
	t.Helper()

	// Queue the settings operation and wait for its processing result.
	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sobject.WaitOperation(ctx, so, localID, account_settings.ProcessAccountSettingsOps); err != nil {
		t.Fatal(err)
	}
}

func decodeAccountSettings(
	ctx context.Context,
	t *testing.T,
	snap sobject.SharedObjectStateSnapshot,
) *account_settings.AccountSettings {
	// Run snapshot-decoding failures at the calling test site.
	t.Helper()

	// Decode the account settings from the supplied snapshot.
	settings, err := account_settings.ReadSnapshot(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func generateTestPeerID(t *testing.T) string {
	// Run key-generation failures at the calling test site.
	t.Helper()

	// Generate a peer keypair for local account tests.
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Derive the test peer ID from its private key.
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid.String()
}

func derivePasswordPeerID(t *testing.T, accountID string, password string) string {
	// Run password-key derivation failures at the calling test site.
	t.Helper()

	// Derive the password key using the requested account identity.
	_, priv, err := auth_password.BuildParametersWithUsernamePassword(accountID, []byte(password))
	if err != nil {
		t.Fatal(err)
	}

	// Derive the peer ID returned by password-key generation.
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
