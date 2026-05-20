//go:build !goscript

package provider_spacewave

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
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

func TestEntityKeyStoreLockAllScrubsKeys(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(time.Hour)
	ref := store.Retain()
	defer ref.Release()
	priv1, pid1, std1 := generateEntityKey(t)
	priv2, pid2, std2 := generateEntityKey(t)

	store.Unlock(pid1, priv1)
	store.Unlock(pid2, priv2)
	assertHasNonZeroKeyBytes(t, std1)
	assertHasNonZeroKeyBytes(t, std2)

	store.LockAll()

	if store.GetUnlockedCount() != 0 {
		t.Fatalf("expected no unlocked keys, got %d", store.GetUnlockedCount())
	}
	assertZeroKeyBytes(t, std1)
	assertZeroKeyBytes(t, std2)
}

func TestEntityKeyStoreUnlockReplacementScrubsOldKey(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(time.Hour)
	ref := store.Retain()
	defer ref.Release()
	priv1, pid, std1 := generateEntityKey(t)
	priv2, _, std2 := generateEntityKey(t)

	store.Unlock(pid, priv1)
	assertHasNonZeroKeyBytes(t, std1)

	store.Unlock(pid, priv2)

	assertZeroKeyBytes(t, std1)
	assertHasNonZeroKeyBytes(t, std2)
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

func generateEntityKey(t *testing.T) (bifrost_crypto.PrivKey, peer.ID, ed25519.PrivateKey) {
	t.Helper()
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("deriving peer ID: %v", err)
	}
	std := priv.(interface{ GetStdKey() ed25519.PrivateKey }).GetStdKey()
	return priv, pid, std
}

func assertHasNonZeroKeyBytes(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	for _, b := range key {
		if b != 0 {
			return
		}
	}
	t.Fatal("expected key bytes to contain non-zero data")
}

func assertZeroKeyBytes(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	for i, b := range key {
		if b != 0 {
			t.Fatalf("expected zeroed key bytes at index %d, got %d", i, b)
		}
	}
}
