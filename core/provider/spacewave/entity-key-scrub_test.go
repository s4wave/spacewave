package provider_spacewave

import (
	"net/http"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
)

// TestProviderRetainEntityKeyBootstrapUpdatesLiveEntityClient verifies
// bootstrap unlock also repairs any running provider account's entity client.
func TestProviderRetainEntityKeyBootstrapUpdatesLiveEntityClient(t *testing.T) {
	var tracker *providerAccountTracker
	p := &Provider{
		httpCli:  http.DefaultClient,
		endpoint: "https://example.com",
	}
	p.accountRc = keyed.NewKeyedRefCount[string, *providerAccountTracker](func(key string) (keyed.Routine, *providerAccountTracker) {
		return nil, tracker
	})
	oldPriv, oldPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)
	acc := &ProviderAccount{
		entityCli:      NewEntityClientDirect(p.httpCli, p.endpoint, DefaultSigningEnvPrefix, oldPriv, oldPID),
		entityKeyStore: p.GetEntityKeyStore("acct-1"),
	}
	tracker = &providerAccountTracker{
		accountID: "acct-1",
		accCtr:    ccontainer.NewCContainer[*ProviderAccount](acc),
	}
	ref, _, _ := p.accountRc.AddKeyRef("acct-1")
	defer ref.Release()

	bootstrapRef := p.RetainEntityKeyBootstrap("acct-1", newPriv, newPID)
	defer bootstrapRef.Release()

	if got := acc.entityCli.peerID; got != newPID {
		t.Fatalf("expected live entity client peer id %q, got %q", newPID, got)
	}
	if acc.entityCli.priv != newPriv {
		t.Fatal("expected live entity client private key to be replaced")
	}
	if !acc.entityKeyStore.IsUnlocked(newPID) {
		t.Fatal("expected bootstrap key to be unlocked in the store")
	}
	if acc.state.selfRejoinSweepGeneration != 1 {
		t.Fatalf("expected bootstrap to trigger self-rejoin generation 1, got %d", acc.state.selfRejoinSweepGeneration)
	}
}
