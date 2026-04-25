package provider_spacewave

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
)

func TestLockSessionInvalidatesFutureMountsAndDropsOwnedSigner(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.sessionClient = NewSessionClient(acc.p.httpCli, acc.p.endpoint, DefaultSigningEnvPrefix, priv, pid.String())
	acc.sessionClientSessionID = "sess-1"

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
		sessionPriv: priv,
		sessionPid:  pid,
		lockMode:    session_lock.SessionLockMode_PIN_ENCRYPTED,
	}

	if err := sess.LockSession(context.Background()); err != nil {
		t.Fatalf("lock session: %v", err)
	}

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
	if acc.GetSessionClient() != nil {
		t.Fatal("expected owned session client to be dropped")
	}
}

func TestGetReadySessionClientRepairsStaleLockedOwner(t *testing.T) {
	stalePriv, stalePID := generateTestKeypair(t)
	freshPriv, freshPID := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.sessionClient = NewSessionClient(acc.p.httpCli, acc.p.endpoint, DefaultSigningEnvPrefix, stalePriv, stalePID.String())
	acc.sessionClientSessionID = "locked"

	lockedProm := promise.NewPromiseContainer[*Session]()
	lockedProm.SetResult(&Session{sessionPid: stalePID}, nil)
	freshProm := promise.NewPromiseContainer[*Session]()
	freshProm.SetResult(&Session{sessionPriv: freshPriv, sessionPid: freshPID}, nil)

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

	cli, priv, pid, err := acc.getReadySessionClient(context.Background())
	if err != nil {
		t.Fatalf("get ready session client: %v", err)
	}
	if cli == nil {
		t.Fatal("expected a repaired session client")
	}
	if priv != freshPriv {
		t.Fatal("expected helper to return the unlocked session private key")
	}
	if pid != freshPID {
		t.Fatal("expected helper to return the unlocked session peer id")
	}
	if acc.sessionClientSessionID != "fresh" {
		t.Fatalf("expected repaired client owner to be fresh, got %q", acc.sessionClientSessionID)
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
		sessionPriv: priv,
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
