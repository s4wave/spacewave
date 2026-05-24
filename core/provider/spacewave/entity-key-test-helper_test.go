package provider_spacewave

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func entityKeyStoreWaitCh(store *EntityKeyStore) <-chan struct{} {
	var ch <-chan struct{}
	store.GetBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	return ch
}

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

func assertHasNonZeroKeyBytes(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	for _, b := range key {
		if b != 0 {
			return
		}
	}
	t.Fatal("expected key bytes to contain non-zero data")
}

func assertZeroKeyBytes(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	for i, b := range key {
		if b != 0 {
			t.Fatalf("expected zeroed key bytes at index %d, got %d", i, b)
		}
	}
}
