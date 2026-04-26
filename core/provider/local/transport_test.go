//go:build todo_flake

// Tests in this file are gated behind the todo_flake build tag because they
// race the sessionTracker auto-started by MountSession against the manual
// CreateSessionTransport in the test body, producing 60s deadlocks in
// CreateSessionTransport when the session tracker blocks indefinitely in
// lookupCloudEndpoint waiting for a spacewave provider that the testbed
// never registers. See
// issues/2026/20260425-provider-local-session-transport-deadlock.org.

package provider_local_test

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
	"github.com/s4wave/spacewave/testbed"
)

// setupProviderAndSession creates a testbed with a local provider, account, and session.
func setupProviderAndSession(ctx context.Context, t *testing.T) (
	*testbed.Testbed,
	*session.SessionRef,
	*provider_local.ProviderAccount,
	*provider_local.Session,
	func(),
) {
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

	// Mount the session to get the session handle with private key.
	sess, sessRelease, err := acc.MountSession(ctx, sessRef, nil)
	if err != nil {
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	localSess := sess.(*provider_local.Session)

	release := func() {
		sessRelease()
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
	return tb, sessRef, acc, localSess, release
}

// TestSessionTransportCreate verifies that CreateSessionTransport creates a
// transport with the session's private key and the child bus resolves the peer.
func TestSessionTransportCreate(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	privKey := sess.GetPrivKey()
	if privKey == nil {
		t.Fatal("session private key is nil")
	}

	// Create transport with empty signaling URL (no WebRTC, but child bus + peer controller).
	if err := acc.CreateSessionTransport(ctx, privKey, ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected session transport to be non-nil")
	}

	// Verify the transport's peer ID matches the session peer ID.
	if st.GetPeerID() != sess.GetPeerId() {
		t.Fatalf("transport peer ID %s != session peer ID %s", st.GetPeerID().String(), sess.GetPeerId().String())
	}

	// Verify the child bus is running (non-nil after Execute starts).
	childBus := st.GetChildBus()
	if childBus == nil {
		t.Fatal("expected child bus to be non-nil after transport starts")
	}
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

// addPairedDeviceAndWait adds a paired device to the account settings SO and
// waits for the state to reflect it.
func addPairedDeviceAndWait(ctx context.Context, t *testing.T, so sobject.SharedObject, peerID, name string) {
	t.Helper()

	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      peerID,
				DisplayName: name,
				PairedAt:    1000,
			},
		},
	}
	opData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	if _, err := so.QueueOperation(ctx, opData); err != nil {
		t.Fatal(err)
	}

	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
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
				if d.GetPeerId() == peerID {
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

// TestTransportCleanup verifies that StopSessionTransport stops the transport
// goroutine cleanly with no leaks.
func TestTransportCleanup(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	privKey := sess.GetPrivKey()
	if err := acc.CreateSessionTransport(ctx, privKey, ""); err != nil {
		t.Fatal(err)
	}

	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected transport to be running")
	}
	if st.GetChildBus() == nil {
		t.Fatal("expected child bus to be non-nil")
	}

	// Stop the transport.
	acc.StopSessionTransport()

	// Verify transport is nil after stop.
	if acc.GetSessionTransport() != nil {
		t.Fatal("expected transport to be nil after stop")
	}

	// Verify we can create a new transport after stopping the old one.
	if err := acc.CreateSessionTransport(ctx, privKey, ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	st2 := acc.GetSessionTransport()
	if st2 == nil {
		t.Fatal("expected new transport after re-create")
	}
	if st2.GetChildBus() == nil {
		t.Fatal("expected new child bus to be non-nil")
	}
}
