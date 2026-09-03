package provider_local

import (
	"context"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// TestReconcilePairingLinksAuthority pins the link-retention authority
// boundary: only durable locally authorized paired-device bindings create
// retained EstablishLinkWithPeer directives. Received SO config grants and
// participants are untrusted hints under current SOSync lineage (a received
// config can be self-authored and is not lineage-verified), so they must
// never produce a directive. Removal of the paired device releases the
// reference on the next reconcile.
func TestReconcilePairingLinksAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	tb, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	_ = tb
	defer release()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()
	st := acc.GetSessionTransport()
	childBus := st.GetChildBus()

	// X is a well-formed but never-connected peer identity.
	xPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	xPriv, err := xPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	xID, err := peer.IDFromPrivateKey(xPriv)
	if err != nil {
		t.Fatal(err)
	}

	settingsRef, err := acc.EnsureAccountSettingsSO(ctx)
	if err != nil {
		t.Fatal(err)
	}
	so, relSO, err := acc.MountSharedObject(ctx, settingsRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relSO()
	localSO, ok := so.(*SharedObject)
	if !ok {
		t.Fatal("settings SO is not a provider-local SharedObject")
	}

	queueOp := func(op *account_settings.AccountSettingsOp) {
		t.Helper()
		opData, err := op.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := so.QueueOperation(ctx, opData); err != nil {
			t.Fatal(err)
		}
	}
	addPaired := func(peerIDStr string) {
		queueOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_AddPairedDevice{
				AddPairedDevice: &account_settings.PairedDevice{
					PeerId:      peerIDStr,
					DisplayName: "authority probe",
					PairedAt:    1000,
				},
			},
		})
	}

	// Untrusted hint: a received-config grant naming an unconnected peer.
	err = localSO.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		state.RootGrants = append(state.RootGrants, &sobject.SOGrant{
			PeerId: xID.String(),
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	state := &p2pSyncState{}
	acc.reconcilePairingLinks(ctx, st, childBus, state)
	if n := len(state.linkRefs); n != 0 {
		t.Fatalf("grant hint produced %d retained link directives; expected 0", n)
	}

	// Owner-signed paired-device binding: authorized.
	addPaired(xID.String())
	acc.reconcilePairingLinks(ctx, st, childBus, state)
	if _, ok := state.linkRefs[xID.String()]; !ok {
		t.Fatal("paired-device binding did not retain a link directive")
	}
	heldBeforeRelease := state.linkRefs[xID.String()] != nil
	if !heldBeforeRelease {
		t.Fatal("retained reference missing")
	}

	// Removal releases the reference on the next reconcile.
	queueOp(&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemovePairedDevice{
			RemovePairedDevice: &account_settings.RemovePairedDeviceOp{
				PeerId: xID.String(),
			},
		},
	})
	acc.reconcilePairingLinks(ctx, st, childBus, state)
	if len(state.linkRefs) != 0 {
		t.Fatalf("unlinked device still holds %d link directives; expected release", len(state.linkRefs))
	}
}
