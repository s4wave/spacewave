package provider_spacewave

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

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
