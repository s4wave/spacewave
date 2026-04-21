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
		var secret [32]byte
		salt := []byte("peer/derive test salt")
		cryptoCtx := fmt.Sprintf("bifrost/peer/derive_test keys[%d]", ki)
		err := DeriveKey(cryptoCtx, salt, key, secret[:])
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("keys[%d]: derived key: %s", ki, b58.Encode(secret[:]))

		// derive private key
		derivPriv, _, err := DeriveEd25519Key(cryptoCtx+" ed25519", salt, key)
		if err == nil && derivPriv == nil {
			err = errors.New("derived empty private key")
		}
		if err != nil {
			t.Fatal(err.Error())
		}
		derivPrivID, err := IDFromPrivateKey(derivPriv)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("keys[%d]: derived private key: %s", ki, derivPrivID.String())
	}
}
