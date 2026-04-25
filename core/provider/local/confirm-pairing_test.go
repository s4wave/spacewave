package provider_local_test

import (
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestConfirmPairingAddsOwner verifies that ConfirmPairing adds the remote
// peer as OWNER on all SharedObjects in the account.
func TestConfirmPairingAddsOwner(t *testing.T) {
	ctx := t.Context()

	_, _, acc, _, release := setupProviderAndSession(ctx, t)
	defer release()

	// Generate a remote peer ID.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerIDStr := remotePeerID.String()

	// The account settings SO should already exist from provider init.
	soList := acc.GetSOListCtr().GetValue()
	if soList == nil || len(soList.GetSharedObjects()) == 0 {
		t.Fatal("expected at least account settings SO in list")
	}

	// Create an additional space SO to test that all SOs get the participant.
	spaceMeta := &sobject.SharedObjectMeta{BodyType: "space"}
	_, err = acc.CreateSharedObject(ctx, "test-space-1", spaceMeta, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the SO list now has 2 entries.
	soList = acc.GetSOListCtr().GetValue()
	if len(soList.GetSharedObjects()) != 2 {
		t.Fatalf("expected 2 SOs, got %d", len(soList.GetSharedObjects()))
	}

	// Confirm pairing.
	if err := acc.ConfirmPairing(ctx, remotePeerID, "Test Device"); err != nil {
		t.Fatal(err)
	}

	// Verify the remote peer is OWNER on all SOs with a grant.
	verifyParticipantOnAllSOs(ctx, t, acc, soList, remotePeerIDStr)

	// Calling ConfirmPairing again should be idempotent (no duplicate participants).
	if err := acc.ConfirmPairing(ctx, remotePeerID, "Test Device"); err != nil {
		t.Fatal(err)
	}

	// Verify still only one entry per SO.
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()
		hostState := getSOState(ctx, t, acc, ref, soID)

		count := 0
		for _, p := range hostState.GetConfig().GetParticipants() {
			if p.GetPeerId() == remotePeerIDStr {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("SO %s: expected 1 remote peer entry, got %d", soID, count)
		}
	}
}

// verifyParticipantOnAllSOs checks that the given peer is OWNER with a grant
// on every SO in the list.
func verifyParticipantOnAllSOs(
	ctx context.Context,
	t *testing.T,
	acc *provider_local.ProviderAccount,
	soList *sobject.SharedObjectList,
	remotePeerIDStr string,
) {
	t.Helper()
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()
		hostState := getSOState(ctx, t, acc, ref, soID)

		// Check participant is OWNER.
		found := false
		for _, p := range hostState.GetConfig().GetParticipants() {
			if p.GetPeerId() == remotePeerIDStr {
				if p.GetRole() != sobject.SOParticipantRole_SOParticipantRole_OWNER {
					t.Fatalf("SO %s: remote peer has role %v, expected OWNER", soID, p.GetRole())
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("SO %s: remote peer not found in participants", soID)
		}

		// Check grant exists.
		grantFound := false
		for _, g := range hostState.GetRootGrants() {
			if g.GetPeerId() == remotePeerIDStr {
				grantFound = true
				break
			}
		}
		if !grantFound {
			t.Fatalf("SO %s: no grant found for remote peer", soID)
		}
	}
}

// TestConfirmPairingPersists verifies that ConfirmPairing writes the paired
// device to the account settings SO via AddPairedDevice operation.
func TestConfirmPairingPersists(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, acc, _, release := setupProviderAndSession(ctx, t)
	defer release()

	// Generate a remote peer ID.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerIDStr := remotePeerID.String()

	// Confirm pairing with a display name.
	if err := acc.ConfirmPairing(ctx, remotePeerID, "My Desktop"); err != nil {
		t.Fatal(err)
	}

	// Mount account settings SO and wait for the paired device to appear.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	so, relSO := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer relSO()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return nil
			}
			rootInner, err := snap.GetRootInner(ctx)
			if err != nil {
				return err
			}
			settings := &account_settings.AccountSettings{}
			if data := rootInner.GetStateData(); len(data) > 0 {
				if err := settings.UnmarshalVT(data); err != nil {
					return err
				}
			}
			for _, d := range settings.GetPairedDevices() {
				if d.GetPeerId() == remotePeerIDStr {
					if d.GetDisplayName() != "My Desktop" {
						t.Errorf("expected display name 'My Desktop', got %q", d.GetDisplayName())
					}
					if d.GetPairedAt() == 0 {
						t.Error("expected non-zero paired_at timestamp")
					}
					return io.EOF
				}
			}
			return nil
		},
		nil,
	)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
}

// TestConfirmPairingStartsSync verifies that ConfirmPairing starts P2P sync
// when the session transport is running.
func TestConfirmPairingStartsSync(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	// Generate a remote peer ID.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}

	// Start the session transport (required for P2P sync).
	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	if acc.IsP2PSyncRunning() {
		t.Fatal("expected no P2P sync before ConfirmPairing")
	}

	// Confirm pairing triggers P2P sync.
	if err := acc.ConfirmPairing(ctx, remotePeerID, "Sync Device"); err != nil {
		t.Fatal(err)
	}

	if !acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to be running after ConfirmPairing")
	}

	acc.StopP2PSync()
}

// TestUnlinkDevice verifies that UnlinkDevice removes the paired device from
// the account settings SO and revokes the peer's SO participant access.
func TestUnlinkDevice(t *testing.T) {
	ctx := t.Context()

	_, _, acc, _, release := setupProviderAndSession(ctx, t)
	defer release()

	// Generate a remote peer ID.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerIDStr := remotePeerID.String()

	// Create an additional space SO.
	spaceMeta := &sobject.SharedObjectMeta{BodyType: "space"}
	_, err = acc.CreateSharedObject(ctx, "test-space-unlink", spaceMeta, "", "")
	if err != nil {
		t.Fatal(err)
	}

	soList := acc.GetSOListCtr().GetValue()
	if len(soList.GetSharedObjects()) != 2 {
		t.Fatalf("expected 2 SOs, got %d", len(soList.GetSharedObjects()))
	}

	// Confirm pairing to add participant + device.
	if err := acc.ConfirmPairing(ctx, remotePeerID, "Unlink Test"); err != nil {
		t.Fatal(err)
	}

	// Verify participant exists on all SOs.
	verifyParticipantOnAllSOs(ctx, t, acc, soList, remotePeerIDStr)

	// Unlink the device.
	if err := acc.UnlinkDevice(ctx, remotePeerID); err != nil {
		t.Fatal(err)
	}

	// Verify participant and grant removed from all SOs.
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()
		hostState := getSOState(ctx, t, acc, ref, soID)

		for _, p := range hostState.GetConfig().GetParticipants() {
			if p.GetPeerId() == remotePeerIDStr {
				t.Fatalf("SO %s: remote peer still in participants after unlink", soID)
			}
		}
		for _, g := range hostState.GetRootGrants() {
			if g.GetPeerId() == remotePeerIDStr {
				t.Fatalf("SO %s: grant still exists for remote peer after unlink", soID)
			}
		}
	}
}

// getSOState mounts an SO and returns its current state.
func getSOState(
	ctx context.Context,
	t *testing.T,
	acc *provider_local.ProviderAccount,
	ref *sobject.SharedObjectRef,
	soID string,
) *sobject.SOState {
	t.Helper()
	so, relSO, err := acc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatalf("mount SO %s: %v", soID, err)
	}
	defer relSO()

	localSO := so.(*provider_local.SharedObject)
	hostState, err := localSO.GetSOHostState(ctx)
	if err != nil {
		t.Fatalf("get host state for SO %s: %v", soID, err)
	}
	return hostState
}
