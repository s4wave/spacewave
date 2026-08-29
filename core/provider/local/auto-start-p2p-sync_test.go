package provider_local_test

import (
	"testing"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
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

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	addPairedDeviceAndWait(ctx, t, so, "12D3KooWAutoStartPaired", "Auto-Start Test Device")
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
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()
	acc.GetSOListCtr().SetValue(&sobject.SharedObjectList{
		SharedObjects: []*sobject.SharedObjectListEntry{{Source: "shared"}},
	})

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
		t.Fatal("expected P2P sync to restart for a joined Space")
	}
}

// _ silences unused import lint when only one of the helpers is referenced.
var _ = provider_local.NewFactory
