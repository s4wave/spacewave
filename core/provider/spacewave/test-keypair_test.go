package provider_spacewave

import (
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// generateTestKeypair generates an Ed25519 keypair for testing.
func generateTestKeypair(t *testing.T) (crypto.PrivKey, peer.ID) {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("deriving peer ID: %v", err)
	}
	return priv, pid
}
