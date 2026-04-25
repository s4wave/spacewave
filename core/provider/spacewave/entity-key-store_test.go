package provider_spacewave

import (
	"testing"
	"time"
)

func TestEntityKeyStoreGraceTimerScrubsAfterLastRef(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(5 * time.Millisecond)
	ref := store.Retain()
	priv, pid, std := generateEntityKey(t)
	store.Unlock(pid, priv)
	assertHasNonZeroKeyBytes(t, std)

	waitForEntityKeyStoreBroadcast(t, store, ref.Release)

	if store.IsUnlocked(pid) {
		t.Fatal("expected grace timer to lock key")
	}
	assertZeroKeyBytes(t, std)
}

func TestEntityKeyStoreRetainsUntilAllRefsRelease(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(5 * time.Millisecond)
	ref1 := store.Retain()
	ref2 := store.Retain()
	priv, pid, std := generateEntityKey(t)
	store.Unlock(pid, priv)

	ref1.Release()
	if !store.IsUnlocked(pid) {
		t.Fatal("expected key to stay unlocked while second ref is retained")
	}
	assertHasNonZeroKeyBytes(t, std)

	waitForEntityKeyStoreBroadcast(t, store, ref2.Release)

	if store.IsUnlocked(pid) {
		t.Fatal("expected key to lock after final ref grace timer")
	}
	assertZeroKeyBytes(t, std)
}

func TestEntityKeyStoreExplicitLockOverridesRefs(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(time.Hour)
	ref := store.Retain()
	defer ref.Release()
	priv, pid, std := generateEntityKey(t)
	store.Unlock(pid, priv)

	store.Lock(pid)

	if store.IsUnlocked(pid) {
		t.Fatal("expected explicit lock to lock key")
	}
	assertZeroKeyBytes(t, std)
}

func TestEntityKeyStoreGraceTimerCancellation(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(5 * time.Millisecond)
	ref1 := store.Retain()
	priv, pid, std := generateEntityKey(t)
	store.Unlock(pid, priv)

	ref1.Release()
	ref2 := store.Retain()
	select {
	case <-entityKeyStoreWaitCh(store):
		t.Fatal("expected retained key to avoid grace timer scrub")
	case <-time.After(20 * time.Millisecond):
	}
	if !store.IsUnlocked(pid) {
		t.Fatal("expected key to stay unlocked after grace timer cancellation")
	}
	assertHasNonZeroKeyBytes(t, std)

	waitForEntityKeyStoreBroadcast(t, store, ref2.Release)

	if store.IsUnlocked(pid) {
		t.Fatal("expected key to lock after final release")
	}
	assertZeroKeyBytes(t, std)
}

func waitForEntityKeyStoreBroadcast(t *testing.T, store *EntityKeyStore, trigger func()) {
	t.Helper()
	ch := entityKeyStoreWaitCh(store)
	trigger()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for entity key store broadcast")
	}
}

func entityKeyStoreWaitCh(store *EntityKeyStore) <-chan struct{} {
	var ch <-chan struct{}
	store.GetBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	return ch
}
