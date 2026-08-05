package peer

import (
	"bytes"
	"testing"
)

// TestEncrypt tests encrypt/decrypt with multiple key types
func TestEncrypt(t *testing.T) {
	// Exercise encryption and decryption for each supported key.
	keys := BuildMockKeys(t)
	for ki, privKey := range keys {
		// Select the public key for this test iteration.
		pubKey := privKey.GetPublic()

		// Derive the peer identity used in the test message.
		peerID, err := IDFromPublicKey(pubKey)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Build the test message and encryption context.
		msg := "Hello to " + peerID.String() + "!"
		context := "bifrost/peer/encrypt_test super-duper-secret"
		dat, err := EncryptToPubKey(pubKey, context, []byte(msg))
		if err != nil {
			t.Fatal(err.Error())
		}

		// Record the ciphertext size for the current key.
		t.Logf(
			"keys[%d]: encrypted: len %d -> %d",
			ki,
			len(msg),
			len(dat),
		)

		// Decrypt the ciphertext with the matching private key.
		dec, err := DecryptWithPrivKey(privKey, context, dat)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Verify that decryption restored the original message.
		if !bytes.Equal(dec, []byte(msg)) {
			t.Fatalf("keys[%d]: data did not match: %v != expected %v", ki, dec, []byte(msg))
		}

		// Record the successful plaintext size.
		t.Logf(
			"keys[%d]: decrypted correctly: len %d",
			ki,
			len(dec),
		)
	}
}
