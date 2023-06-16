package auth_method_triplesec

import (
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
)

// TestBasicAuthenticate tests deterministic key derivation.
func TestBasicAuthenticate(t *testing.T) {
	method := NewTriplesecPassword()
	params, _, err := BuildParametersWithUsernamePassword(4, "test username", []byte("test password"))
	if err != nil {
		t.Fatal(err.Error())
	}
	privKey, err := method.Authenticate(params, []byte("test password"))
	if err != nil {
		t.Fatal(err.Error())
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	peerIDPretty := peerID.Pretty()
	t.Log(peerIDPretty)

	// determinism check
	expectedPeerID := "12D3KooWNZcv3LvXM27oom23NN1CxiQHYFMXGC1gAmgcgLwsbocc"
	if peerIDPretty != expectedPeerID {
		t.Fatalf("expected peer ID %s", expectedPeerID)
	}
}
