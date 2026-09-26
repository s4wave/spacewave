package peer

import (
	"crypto/ed25519"
	"errors"
	"testing"
)

// TestDecryptWithEd25519ShortCiphertext tests that truncated ciphertexts are rejected.
func TestDecryptWithEd25519ShortCiphertext(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, n := range []int{0, 4, 34, 35, 36, 51} {
		_, err := DecryptWithEd25519(privKey, "short-test", make([]byte, n))
		if !errors.Is(err, ErrShortMessage) {
			t.Fatalf("len %d: expected ErrShortMessage, got %v", n, err)
		}
	}
}
