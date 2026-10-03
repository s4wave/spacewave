package volume_rpc_server

import (
	"encoding/hex"
	"testing"
)

func TestCoordinatorLeaseIDsAreOpaque(t *testing.T) {
	// Create a lease registry and acquire its first opaque identifier.
	leases := newCoordinatorLeases()
	first, err := leases.add(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Acquire another lease identifier from the same registry.
	second, err := leases.add(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that separate leases receive distinct identifiers.
	if first == second {
		t.Fatal("coordinator lease IDs collided")
	}

	// Verify that both identifiers contain sixteen opaque bytes.
	for _, id := range []string{first, second} {
		// Decode the lease identifier as hexadecimal bytes.
		decoded, err := hex.DecodeString(id)
		if err != nil {
			t.Fatalf("lease ID is not opaque hexadecimal: %q: %v", id, err)
		}

		// Verify the lease identifier retains the full random token.
		if len(decoded) != 16 {
			t.Fatalf("lease ID has %d bytes, want 16", len(decoded))
		}
	}
}
