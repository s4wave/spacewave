package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/session"
)

func TestMountedPINUnlockRestoresLocalSessionState(t *testing.T) {
	ctx := t.Context()
	_, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	pin := []byte("2468")
	if err := sess.SetLockMode(ctx, session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED, pin); err != nil {
		t.Fatalf("set PIN lock mode: %v", err)
	}
	if err := sess.LockSession(ctx); err != nil {
		t.Fatalf("lock session: %v", err)
	}
	if sess.GetPrivKey() != nil {
		t.Fatal("expected locked session private key to be cleared")
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	lockEvents := make(chan bool, 8)
	watchErr := make(chan error, 1)
	go func() {
		err := sess.WatchLockState(watchCtx, func(mode session.SessionLockMode, locked bool) {
			if mode != session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
				return
			}
			select {
			case lockEvents <- locked:
			default:
			}
		})
		if err != nil && err != context.Canceled {
			watchErr <- err
		}
	}()
	waitForLockState(t, lockEvents, watchErr, true)

	if err := sess.UnlockSession(ctx, []byte("wrong")); err == nil {
		t.Fatal("expected wrong PIN to fail mounted unlock")
	}
	if sess.GetPrivKey() != nil {
		t.Fatal("wrong PIN restored the session private key")
	}

	if err := sess.UnlockSession(ctx, pin); err != nil {
		t.Fatalf("unlock mounted session: %v", err)
	}
	waitForLockState(t, lockEvents, watchErr, false)
	if sess.GetPrivKey() == nil {
		t.Fatal("expected mounted unlock to restore the session private key")
	}

	mountCtx, mountCancel := context.WithTimeout(ctx, 5*time.Second)
	defer mountCancel()
	mounted, mountedRelease, err := acc.MountSession(mountCtx, sessRef, nil)
	if err != nil {
		t.Fatalf("mount session after mounted unlock: %v", err)
	}
	defer mountedRelease()
	if mounted.GetPrivKey() == nil {
		t.Fatal("expected future mount to receive unlocked session")
	}
}

func waitForLockState(t *testing.T, events <-chan bool, errCh <-chan error, want bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		select {
		case got := <-events:
			if got == want {
				return
			}
		case err := <-errCh:
			t.Fatalf("watch lock state: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for lock state %v", want)
		}
	}
}
