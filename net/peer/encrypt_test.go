package peer

import (
	"bytes"
	"testing"
)

// TestEncrypt tests encrypt/decrypt with multiple key types
func TestEncrypt(t *testing.T) {
	keys := BuildMockKeys(t)
	for ki, privKey := range keys {
		pubKey := privKey.GetPublic()

		peerID, err := IDFromPublicKey(pubKey)
		if err != nil {
			t.Fatal(err.Error())
		}

		// super-secret message
		msg := "Hello to " + peerID.String() + "!"
		context := "bifrost/peer/encrypt_test super-duper-secret"
		dat, err := EncryptToPubKey(pubKey, context, []byte(msg))
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf(
			"keys[%d]: encrypted: len %d -> %d",
			ki,
			len(msg),
			len(dat),
		)

		dec, err := DecryptWithPrivKey(privKey, context, dat)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !bytes.Equal(dec, []byte(msg)) {
			t.Fatalf("keys[%d]: data did not match: %v != expected %v", ki, dec, []byte(msg))
		}
		t.Logf(
			"keys[%d]: decrypted correctly: len %d",
			ki,
			len(dec),
		)
	}
}
