package volume_rpc_server

import (
	"encoding/hex"
	"testing"
)

func TestCoordinatorLeaseIDsAreOpaque(t *testing.T) {
	leases := newCoordinatorLeases()
	first, err := leases.add(nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := leases.add(nil)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("coordinator lease IDs collided")
	}
	for _, id := range []string{first, second} {
		decoded, err := hex.DecodeString(id)
		if err != nil {
			t.Fatalf("lease ID is not opaque hexadecimal: %q: %v", id, err)
		}
		if len(decoded) != 16 {
			t.Fatalf("lease ID has %d bytes, want 16", len(decoded))
		}
	}
}
