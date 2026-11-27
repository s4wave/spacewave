package provider_local

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
)

// TestDeleteStateOffKeepsOldBehavior selects the old behavior and checks that
// delete keeps the stored state, create of the same id adopts it, and a mount
// of an unlisted id with no state writes a genesis.
func TestDeleteStateOffKeepsOldBehavior(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	setTestDeleteStateOff(t, true)

	// Give an object state and delete it: the state stays.
	ref := createTestSharedObject(ctx, t, a, "oldway")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	writeTestStateData(ctx, t, so, []byte("fake-kept"))
	rel()
	keys := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("oldway"))
	if err := a.DeleteSharedObject(ctx, "oldway"); err != nil {
		t.Fatal(err)
	}
	if testListed(a, "oldway") {
		t.Fatal("delete left the object listed")
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("oldway")); n != keys {
		t.Fatalf("delete under the old behavior left %d keys, want %d", n, keys)
	}

	// Create of the same id writes only the list and adopts the kept state.
	ref = createTestSharedObject(ctx, t, a, "oldway")
	so, rel = mountTestSharedObject(ctx, t, a, ref)
	if got := readTestStateData(ctx, t, so); !bytes.Equal(got, []byte("fake-kept")) {
		t.Fatalf("the recreated object reads %q", got)
	}
	rel()

	// A mount of an unlisted id with no state writes a genesis.
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	ghost := sobject.NewSharedObjectRef(providerID, accountID, "unlisted", SobjectBlockStoreID("unlisted"))
	_, rel = mountTestSharedObject(ctx, t, a, ghost)
	rel()
}
