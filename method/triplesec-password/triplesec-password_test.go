package auth_method_triplesecpassword

import (
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/keybase/go-triplesec"
	"golang.org/x/crypto/blake2s"
)

// TestBasicAuthenticate tests deterministic key derivation.
func TestBasicAuthenticate(t *testing.T) {
	method := NewTriplesecPassword()
	salt := blake2s.Sum256([]byte("username"))
	privKey, err := method.Authenticate(&Parameters{
		Salt:    salt[:triplesec.SaltLen],
		Version: 4,
	}, []byte("password"))
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
	expectedPeerID := "12D3KooWQ5r25i8wnUtXbB4rrRj5sqy51rt58NAvLcjEzcVBbAEB"
	if peerIDPretty != expectedPeerID {
		t.Fatalf("expected peer ID %s", expectedPeerID)
	}
}
