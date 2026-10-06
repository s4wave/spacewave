package provider_local

import (
	"context"
	"testing"
	"time"
)

// TestSessionTransportSurvivesConsumerHandoff exercises the temporary pairing
// mount ending before the registered Session's account view mounts it again.
func TestSessionTransportSurvivesConsumerHandoff(t *testing.T) {
	// Bound the handoff and mount the first session consumer.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, ref, account, first, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Establish readiness before ending the first Session consumer.
	if err := account.EnsureConfiguredSessionTransport(ctx, first.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	transport := account.GetSessionTransport()
	_, exited := first.tkr.sessionProm.GetPromise()
	if !account.sessions.RemoveKey(first.tkr.id) {
		t.Fatal("first Session consumer was not mounted")
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Reopening the persisted Session retains its live transport and identity.
	next, releaseNext, err := account.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseNext()
	if err := account.EnsureConfiguredSessionTransport(ctx, next.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if next.GetPeerId() != first.GetPeerId() || account.GetSessionTransport() != transport {
		t.Fatal("consumer handoff replaced the Session identity or transport")
	}
}

// TestPINLockStopsSessionTransport verifies that an account-owned connection
// cannot keep authenticating with a credential after the user locks it.
func TestPINLockStopsSessionTransport(t *testing.T) {
	// Bound the test and start the configured session transport.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, account, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	if err := account.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
		t.Fatal(err)
	}

	// Lock the session and require its credential and transport to clear.
	before := account.GetSessionTransport()
	pin := []byte("2468")
	configureLowCostPINLock(ctx, t, sess, pin)
	if err := sess.LockSession(ctx); err != nil {
		t.Fatal(err)
	}
	if sess.GetPrivKey() != nil || account.GetSessionTransport() != nil {
		t.Fatal("locking retained the Session credential or its transport")
	}

	// Unlock the session and require a fresh authorized transport.
	if err := sess.UnlockSession(ctx, pin); err != nil {
		t.Fatal(err)
	}
	if account.GetSessionTransport() == nil || account.GetSessionTransport() == before {
		t.Fatal("unlocking did not establish a fresh authorized transport")
	}
}
