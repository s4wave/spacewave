package provider_local

import (
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// verifyParticipantOnAllSOs checks that the given peer is OWNER with a grant
// on every SO in the list.
func verifyParticipantOnAllSOs(
	ctx context.Context,
	t *testing.T,
	acc *ProviderAccount,
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

// TestRecordPairedDevicePersists verifies the durable reconnect record used by
// account pairing and managed Device enrollment.
func TestRecordPairedDevicePersists(t *testing.T) {
	ctx := t.Context()

	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t)
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

	if err := acc.RecordPairedDevice(ctx, remotePeerID.String(), "My Desktop"); err != nil {
		t.Fatal(err)
	}

	accountSettingsRef, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	so, relSO, err := acc.MountSharedObject(ctx, accountSettingsRef, nil)
	if err != nil {
		t.Fatal(err)
	}
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

// TestUnlinkDevice verifies that UnlinkDevice removes the paired device from
// the account settings SO and revokes the peer's SO participant access.
func TestUnlinkDevice(t *testing.T) {
	// Start a local provider account and session.
	ctx := t.Context()
	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t)
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

	// Arrange a managed Device's existing owner grants and reconnect record.
	storagePeer, err := acc.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	storageKey, err := storagePeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range soList.GetSharedObjects() {
		so, releaseSO, err := acc.MountSharedObject(ctx, entry.GetRef(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = sobject.AddSOParticipant(ctx, so.(*SharedObject).soHost, entry.GetRef().GetProviderResourceRef().GetId(), storageKey, storagePeer.GetPeerID().String(), remotePeerIDStr, remotePriv.GetPublic(), sobject.SOParticipantRole_SOParticipantRole_OWNER, "", "")
		releaseSO()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := acc.RecordPairedDevice(ctx, remotePeerIDStr, "Unlink Test"); err != nil {
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
	acc *ProviderAccount,
	ref *sobject.SharedObjectRef,
	soID string,
) *sobject.SOState {
	t.Helper()
	so, relSO, err := acc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatalf("mount SO %s: %v", soID, err)
	}
	defer relSO()

	localSO := so.(*SharedObject)
	hostState, err := localSO.GetSOHostState(ctx)
	if err != nil {
		t.Fatalf("get host state for SO %s: %v", soID, err)
	}
	return hostState
}
