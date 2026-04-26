//go:build todo_flake

// See issues/2026/20260425-provider-local-session-transport-deadlock.org.
// Gated behind todo_flake because setupProviderAndSession races the
// sessionTracker against the test body and deadlocks under load.

package provider_local_test

import (
	"context"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
)

// TestAutoStartP2PSyncIfPairedNoDevices verifies the auto-start helper is a
// no-op when no paired devices have been recorded. It must NOT spin up
// P2P sync controllers for accounts that never paired.
func TestAutoStartP2PSyncIfPairedNoDevices(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

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

	if err := acc.AutoStartP2PSyncIfPaired(ctx, st); err != nil {
		t.Fatalf("AutoStartP2PSyncIfPaired: %v", err)
	}

	if acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to be idle when no paired devices recorded")
	}
}

// TestAutoStartP2PSyncIfPairedWithDevice verifies that AutoStartP2PSyncIfPaired
// calls StartP2PSync when AccountSettings has at least one paired device,
// proving the session-mount path will resume P2P sync after a remount with
// a paired peer.
func TestAutoStartP2PSyncIfPairedWithDevice(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

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

	if err := acc.AutoStartP2PSyncIfPaired(ctx, st); err != nil {
		t.Fatalf("AutoStartP2PSyncIfPaired: %v", err)
	}
	defer acc.StopP2PSync()

	if !acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to be running after auto-start with paired device present")
	}
}

// _ silences unused import lint when only one of the helpers is referenced.
var _ = provider_local.NewFactory
