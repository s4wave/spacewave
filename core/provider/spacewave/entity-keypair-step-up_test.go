package provider_spacewave

import (
	"testing"
	"time"

	"github.com/aperturerobotics/util/refcount"
)

// TestEntityKeypairStepUpReleaseUsesStoreGrace verifies the refcounted step-up
// release path lets the store grace timer scrub keypairs.
func TestEntityKeypairStepUpReleaseUsesStoreGrace(t *testing.T) {
	acc := &ProviderAccount{
		entityKeyStore: NewEntityKeyStoreWithGrace(5 * time.Millisecond),
	}
	//nolint:staticcheck // test uses a fixed parent context.
	acc.entityKeypairStepUpRc = refcount.NewRefCount(
		t.Context(),
		false,
		nil,
		nil,
		acc.resolveEntityKeypairStepUp,
	)

	priv, pid, std := generateEntityKey(t)
	acc.entityKeyStore.Unlock(pid, priv)
	assertHasNonZeroKeyBytes(t, std)

	_, release, err := acc.entityKeypairStepUpRc.Resolve(t.Context())
	if err != nil {
		t.Fatalf("resolve step-up retention: %v", err)
	}
	ch := entityKeyStoreWaitCh(acc.entityKeyStore)
	release()

	if !acc.entityKeyStore.IsUnlocked(pid) {
		t.Fatal("expected step-up release to leave the keypair unlocked during grace")
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for step-up grace scrub")
	}
	if acc.entityKeyStore.IsUnlocked(pid) {
		t.Fatal("expected step-up release to scrub the keypair after grace")
	}
	assertZeroKeyBytes(t, std)
}

// TestEntityKeypairStepUpReleasePreservesBootstrapRef verifies step-up release
// does not bypass another active store retention ref.
func TestEntityKeypairStepUpReleasePreservesBootstrapRef(t *testing.T) {
	acc := &ProviderAccount{
		entityKeyStore: NewEntityKeyStoreWithGrace(time.Hour),
	}
	//nolint:staticcheck // test uses a fixed parent context.
	acc.entityKeypairStepUpRc = refcount.NewRefCount(
		t.Context(),
		false,
		nil,
		nil,
		acc.resolveEntityKeypairStepUp,
	)

	priv, pid, std := generateEntityKey(t)
	acc.entityKeyStore.Unlock(pid, priv)
	bootstrapRef := acc.entityKeyStore.Retain()
	defer bootstrapRef.Release()
	assertHasNonZeroKeyBytes(t, std)

	_, release, err := acc.entityKeypairStepUpRc.Resolve(t.Context())
	if err != nil {
		t.Fatalf("resolve step-up retention: %v", err)
	}
	release()

	if !acc.entityKeyStore.IsUnlocked(pid) {
		t.Fatal("expected bootstrap ref to preserve the keypair")
	}
	assertHasNonZeroKeyBytes(t, std)
}
