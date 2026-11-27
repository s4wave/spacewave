package provider_spacewave

import (
	"context"
	"net/http"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/net/crypto"
)

// TestLockSessionInvalidatesFutureMountsAndDropsOwnedSigner verifies lock revokes retained clients and releases the Session.
func TestLockSessionInvalidatesFutureMountsAndDropsOwnedSigner(t *testing.T) {
	// Install and retain the Session's signing client.
	priv, pid := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.maybeSetSessionClient("sess-1", priv, pid)
	retained := acc.currentSessionClient()

	// Construct the mounted Session and its release hook.
	sessionProm := promise.NewPromiseContainer[*Session]()
	sessionProm.SetResult(&Session{}, nil)
	unlockProm := promise.NewPromiseContainer[[]byte]()
	released := 0
	tkr := &sessionTracker{
		a:                  acc,
		id:                 "sess-1",
		sessionProm:        sessionProm,
		unlockProm:         unlockProm,
		releasePinnedRefFn: func() { released++ },
	}
	sess := &Session{
		tkr:         tkr,
		sessionPriv: newSessionCredential(context.Background(), priv),
		sessionPid:  pid,
		lockMode:    session_lock.SessionLockMode_PIN_ENCRYPTED,
	}

	// Lock the Session through its production lifecycle.
	if err := sess.LockSession(context.Background()); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	// Require lock to clear future mounts and release the pinned tracker.
	if sess.sessionPriv != nil {
		t.Fatal("expected session private key to be scrubbed")
	}
	if prom, _ := sessionProm.GetPromise(); prom != nil {
		t.Fatal("expected session promise to be cleared")
	}
	if tkr.unlockProm == unlockProm {
		t.Fatal("expected unlock promise to be replaced")
	}
	if released != 1 {
		t.Fatalf("expected pinned ref release once, got %d", released)
	}
	if _, err := acc.GetSessionClient(); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("expected signing gap after lock, got %v", err)
	}

	// A retained client loses its key and refuses all future request signatures.
	signer := retained.GetSessionSigner()
	select {
	case <-signer.Done():
	default:
		t.Fatal("expected retained signer revocation notification")
	}
	if signer.CanSign() || signer.PrivKey() != nil {
		t.Fatal("retained signer still holds a live key")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/api/account/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := retained.signRequest(req, nil); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("retained client signed after lock: %v", err)
	}
}

// TestGetReadySessionClientInstallsAfterOwnerLocks verifies repair installs a newer signer after lock clears the slot.
func TestGetReadySessionClientInstallsAfterOwnerLocks(t *testing.T) {
	// Install and revoke the old owner exactly as LockSession does.
	stalePriv, stalePID := generateTestKeypair(t)
	freshPriv, freshPID := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.maybeSetSessionClient("locked", stalePriv, stalePID)
	retained := acc.currentSessionClient()
	acc.dropSessionClientForSession("locked")

	// Mount one locked Session and one unlocked Session for repair.
	lockedProm := promise.NewPromiseContainer[*Session]()
	lockedProm.SetResult(&Session{sessionPid: stalePID}, nil)
	freshProm := promise.NewPromiseContainer[*Session]()
	freshProm.SetResult(&Session{tkr: &sessionTracker{a: acc, id: "fresh"}, sessionPriv: newSessionCredential(context.Background(), freshPriv), sessionPid: freshPID}, nil)

	// Expose both mounted Sessions to the account repair path.
	trackers := map[string]*sessionTracker{
		"locked": {sessionProm: lockedProm},
		"fresh":  {sessionProm: freshProm},
	}
	acc.sessions = keyed.NewKeyedRefCount[string, *sessionTracker](
		func(key string) (keyed.Routine, *sessionTracker) {
			return nil, trackers[key]
		},
	)
	for key := range trackers {
		ref, _, _ := acc.sessions.AddKeyRef(key)
		defer ref.Release()
	}

	// Capture the wait channel after the old owner dropped its signer.
	var readyCh <-chan struct{}
	acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		readyCh = getWaitCh()
	})

	// Require repair to select the unlocked Session and wake waiters.
	cli, priv, pid, err := acc.getReadySessionClient(context.Background())
	if err != nil {
		t.Fatalf("get ready session client: %v", err)
	}
	if cli == nil {
		t.Fatal("expected a repaired session client")
	}
	if !priv.Equals(freshPriv) {
		t.Fatal("expected helper to return the unlocked session private key")
	}
	if pid != freshPID {
		t.Fatal("expected helper to return the unlocked session peer id")
	}
	if acc.sessionClientSessionID != "fresh" {
		t.Fatalf("expected repaired client owner to be fresh, got %q", acc.sessionClientSessionID)
	}
	select {
	case <-readyCh:
	default:
		t.Fatal("expected repaired session client to wake account waiters")
	}

	// The fresh signer advances the generation without reviving the old client.
	if cli.GetSessionSigner().GetGeneration() <= retained.GetSessionSigner().GetGeneration() {
		t.Fatal("fresh signer did not advance the generation")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/api/account/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := retained.signRequest(req, nil); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("retained locked client signed after repair: %v", err)
	}
}

// TestMaybeSetSessionClientKeepsLiveSigner verifies repeated lookups preserve a live capability without broadcasting.
func TestMaybeSetSessionClientKeepsLiveSigner(t *testing.T) {
	// Retain the first Session's signer and its next slot notification.
	priv, pid := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.maybeSetSessionClient("first", priv, pid)
	signer := acc.currentSessionClient().GetSessionSigner()
	generation := signer.GetGeneration()
	var changed <-chan struct{}
	acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		changed = getWaitCh()
	})

	// Reinstalling the same Session keeps the signer live without waking waiters.
	acc.maybeSetSessionClient("first", priv, pid)
	if got := acc.currentSessionClient().GetSessionSigner(); got != signer || !got.CanSign() || got.GetGeneration() != generation {
		t.Fatal("same Session replaced or revoked its live signer")
	}
	select {
	case <-signer.Done():
		t.Fatal("same Session revoked its live signer")
	default:
	}
	select {
	case <-changed:
		t.Fatal("same Session broadcast without changing its signer")
	default:
	}

	// Another Session cannot take over the first Session's live signer.
	otherPriv, otherPID := generateTestKeypair(t)
	acc.maybeSetSessionClient("other", otherPriv, otherPID)
	if acc.currentSessionClient().GetSessionSigner() != signer || acc.sessionClientSessionID != "first" {
		t.Fatal("another Session replaced the live slot owner")
	}
}

// TestLockSessionRevokesReauthenticatedSigner verifies that the signer
// reauthentication installs belongs to its Session, so the Session's lock
// revokes it.
func TestLockSessionRevokesReauthenticatedSigner(t *testing.T) {
	// Reauthenticate a Session with a fresh key and retain its client.
	oldPriv, oldPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.maybeSetSessionClient("sess-1", oldPriv, oldPID)
	acc.ReplaceSessionClient("sess-1", newPriv, newPID)
	retained := acc.currentSessionClient()
	signer := retained.GetSessionSigner()

	// The tracker's re-registration with the same key keeps that signer.
	acc.maybeSetSessionClient("sess-1", newPriv, newPID)
	if acc.currentSessionClient().GetSessionSigner() != signer {
		t.Fatal("re-registration replaced the reauthenticated signer")
	}

	// Lock the Session as LockSession does and require the signer revoked.
	acc.dropSessionClientForSession("sess-1")
	if signer.CanSign() {
		t.Fatal("reauthenticated signer survived the Session's lock")
	}
	if _, err := acc.GetSessionClient(); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("expected signing gap after lock, got %v", err)
	}
}

func TestLockSessionDropsPinnedTrackerSoNextMountGetsFreshTracker(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.sessions = keyed.NewKeyedRefCount[string, *sessionTracker](acc.buildSessionTracker)

	mountRef, tkr, existed := acc.sessions.AddKeyRef("sess-1")
	if existed {
		t.Fatal("expected first session tracker ref to create a new tracker")
	}
	pinnedRef, _, existed := acc.sessions.AddKeyRef("sess-1")
	if !existed {
		t.Fatal("expected pinned ref to reuse the existing tracker")
	}
	tkr.sessionProm.SetResult(&Session{}, nil)
	tkr.releasePinnedRefFn = pinnedRef.Release

	sess := &Session{
		tkr:         tkr,
		sessionPriv: newSessionCredential(context.Background(), priv),
		sessionPid:  pid,
		lockMode:    session_lock.SessionLockMode_PIN_ENCRYPTED,
	}

	if err := sess.LockSession(context.Background()); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	mountRef.Release()

	if keys := acc.sessions.GetKeys(); len(keys) != 0 {
		t.Fatalf("expected locked tracker to be dropped after last external release, got keys %v", keys)
	}

	nextRef, nextTkr, existed := acc.sessions.AddKeyRef("sess-1")
	defer nextRef.Release()
	if existed {
		t.Fatal("expected next mount to construct a fresh tracker")
	}
	if nextTkr == tkr {
		t.Fatal("expected next mount to receive a different tracker instance")
	}
}

// BenchmarkSessionCredentialSign measures the complete synchronous key fence.
func BenchmarkSessionCredentialSign(b *testing.B) {
	// Compare the same Ed25519 key and payload with and without revocation fencing.
	priv, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, -1)
	if err != nil {
		b.Fatal(err)
	}
	credential := newSessionCredential(b.Context(), priv)
	defer credential.revoke()
	payload := []byte("session credential signing cost")
	signer := newSessionSigner("benchmark", "", 1, credential, nil)
	b.Run("request-lifetime", func(b *testing.B) {
		// Measure a signed request's context binding and completed-body cleanup.
		b.ReportAllocs()
		for b.Loop() {
			_, release := signer.bindRequestContext(context.Background())
			release()
		}
	})

	for _, item := range []struct {
		// name identifies the measured key path.
		name string
		// key signs the fixed payload on that path.
		key crypto.PrivKey
	}{{"raw", priv}, {"fenced", credential}} {
		b.Run(item.name, func(b *testing.B) {
			// Measure only synchronous signing, excluding setup and cleanup.
			b.ReportAllocs()
			for b.Loop() {
				if _, err := item.key.Sign(payload); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
