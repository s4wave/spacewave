package peer

import (
	"errors"
	"fmt"
	"testing"

	b58 "github.com/mr-tron/base58/base58"
)

// TestDerive tests deriving a context-specific key from crypto keys.
func TestDerive(t *testing.T) {
	keys := BuildMockKeys(t)
	for ki, key := range keys {
		// Prepare the salt and context for this key.
		var secret [32]byte
		salt := []byte("peer/derive test salt")
		cryptoCtx := fmt.Sprintf("bifrost/peer/derive_test keys[%d]", ki)

		// Derive and record the symmetric secret.
		err := DeriveKey(cryptoCtx, salt, key, secret[:])
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("keys[%d]: derived key: %s", ki, b58.Encode(secret[:]))

		// Derive an Ed25519 key from the same context.
		derivPriv, _, err := DeriveEd25519Key(cryptoCtx+" ed25519", salt, key)
		if err == nil && derivPriv == nil {
			err = errors.New("derived empty private key")
		}
		if err != nil {
			t.Fatal(err.Error())
		}

		// Derive the peer identity of the new private key.
		derivPrivID, err := IDFromPrivateKey(derivPriv)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("keys[%d]: derived private key: %s", ki, derivPrivID.String())
	}
}

// TestDeriveEmptyContext tests that DeriveKey rejects an empty context.
func TestDeriveEmptyContext(t *testing.T) {
	keys := BuildMockKeys(t)

	// Derive with an empty context; expect ErrEmptyContext.
	var secret [32]byte
	err := DeriveKey("", nil, keys[0], secret[:])
	if !errors.Is(err, ErrEmptyContext) {
		t.Fatalf("expected ErrEmptyContext, got: %v", err)
	}
}
