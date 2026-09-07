package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/link"
)

// TestAutoStartP2PSyncIfNeededNoDevices verifies the auto-start helper is a
// no-op when no paired devices have been recorded. It must NOT spin up
// P2P sync controllers for accounts that never paired.
func TestAutoStartP2PSyncIfNeededNoDevices(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}
	defer acc.StopSessionTransport()

	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected non-nil session transport")
	}

	if err := acc.AutoStartP2PSyncIfNeeded(ctx, st); err != nil {
		t.Fatalf("AutoStartP2PSyncIfNeeded: %v", err)
	}

	if acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to be idle when no paired devices recorded")
	}
}

// TestAutoStartP2PSyncIfNeededWithDevice verifies that AutoStartP2PSyncIfNeeded
// calls StartP2PSync when AccountSettings has at least one paired device,
// proving the session-mount path will resume P2P sync after a remount with
// a paired peer.
func TestAutoStartP2PSyncIfNeededWithDevice(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()
	_, _, _, remoteSess, releaseRemote := setupProviderAndSession(ctx, t)
	defer releaseRemote()

	remotePeerID := remoteSess.GetPeerId().String()
	if err := acc.RecordPairedDevice(ctx, remotePeerID, "Auto-Start Test Device"); err != nil {
		t.Fatalf("RecordPairedDevice: %v", err)
	}
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	waitPairedDevice(ctx, t, so, remotePeerID)
	soRelease()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}
	defer acc.StopSessionTransport()
	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected non-nil session transport")
	}
	if err := acc.AutoStartP2PSyncIfNeeded(ctx, st); err != nil {
		t.Fatalf("AutoStartP2PSyncIfNeeded: %v", err)
	}
	defer acc.StopP2PSync()
	if !acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to be running after auto-start with paired device present")
	}
}

// TestAutoStartP2PSyncIfNeededWithSharedSpace verifies that a Device account
// restores P2P sync after restart even though invite enrollment does not add a
// paired-device record.
func TestAutoStartP2PSyncIfNeededWithSharedSpace(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()
	_, _, _, remoteSess, releaseRemote := setupProviderAndSession(ctx, t)
	defer releaseRemote()
	remotePeerID := remoteSess.GetPeerId().String()

	spaceRef, err := acc.CreateSharedObject(ctx, "auto-start-shared-space", &sobject.SharedObjectMeta{
		BodyType: "space",
	}, "", "")
	if err != nil {
		t.Fatalf("CreateSharedObject: %v", err)
	}
	soList := acc.GetSOListCtr().GetValue().CloneVT()
	for _, entry := range soList.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == spaceRef.GetProviderResourceRef().GetId() {
			entry.Source = "shared"
			entry.TransportPeerId = remotePeerID
		}
	}
	acc.GetSOListCtr().SetValue(soList)

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}
	defer acc.StopSessionTransport()
	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected non-nil session transport")
	}
	linkTargets := make(chan string, 1)
	removeHandler, err := st.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			establish, ok := di.GetDirective().(link.EstablishLinkWithPeer)
			if !ok {
				return nil, nil
			}
			select {
			case linkTargets <- establish.EstablishLinkTargetPeerId().String():
			default:
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatalf("add link directive handler: %v", err)
	}
	defer removeHandler()

	startCtx, cancelStart := context.WithCancel(ctx)
	if err := acc.AutoStartP2PSyncIfNeeded(startCtx, st); err != nil {
		t.Fatalf("AutoStartP2PSyncIfNeeded: %v", err)
	}
	defer acc.StopP2PSync()
	cancelStart()
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to outlive the session-mount startup context")
	}
	select {
	case target := <-linkTargets:
		if target != remotePeerID {
			t.Fatalf("auto-start link target = %s, want saved transport peer %s", target, remotePeerID)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for auto-start link to %s", remotePeerID)
	}
}

// _ silences unused import lint when only one of the helpers is referenced.
var _ = provider_local.NewFactory
