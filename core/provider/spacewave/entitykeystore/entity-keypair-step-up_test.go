package entitykeystore

import (
	"testing"
	"time"
)

func TestEntityKeypairStepUpReleaseUsesStoreGrace(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(5 * time.Millisecond)
	stepUp := NewEntityKeypairStepUp(t.Context(), func() *EntityKeyStore {
		return store
	})
	priv, pid, std := generateEntityKey(t)
	store.Unlock(pid, priv)
	assertHasNonZeroKeyBytes(t, std)

	_, release, err := stepUp.Resolve(t.Context())
	if err != nil {
		t.Fatalf("resolve step-up retention: %v", err)
	}
	waitCh := entityKeyStoreWaitCh(store)
	release()

	if !store.IsUnlocked(pid) {
		t.Fatal("expected step-up release to leave the keypair unlocked during grace")
	}
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for step-up grace scrub")
	}
	if store.IsUnlocked(pid) {
		t.Fatal("expected step-up release to scrub the keypair after grace")
	}
	assertZeroKeyBytes(t, std)
}

func TestEntityKeypairStepUpReleasePreservesBootstrapRef(t *testing.T) {
	store := NewEntityKeyStoreWithGrace(5 * time.Millisecond)
	stepUp := NewEntityKeypairStepUp(t.Context(), func() *EntityKeyStore {
		return store
	})
	priv, pid, std := generateEntityKey(t)
	store.Unlock(pid, priv)
	bootstrapRef := store.Retain()
	assertHasNonZeroKeyBytes(t, std)

	_, release, err := stepUp.Resolve(t.Context())
	if err != nil {
		t.Fatalf("resolve step-up retention: %v", err)
	}
	release()

	if !store.IsUnlocked(pid) {
		t.Fatal("expected bootstrap ref to preserve the keypair")
	}
	assertHasNonZeroKeyBytes(t, std)

	waitForEntityKeyStoreBroadcast(t, store, bootstrapRef.Release)
	if store.IsUnlocked(pid) {
		t.Fatal("expected final bootstrap release to scrub the keypair")
	}
	assertZeroKeyBytes(t, std)
}
