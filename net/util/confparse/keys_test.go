package confparse

import (
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestParseKeys tests parsing public and private key pems.
func TestParseKeys(t *testing.T) {
	testKeyTypes(t, testParseKeys)
}

func testParseKeys(t *testing.T, keyPriv crypto.PrivKey, keyPub crypto.PubKey) {
	// Encode the private key as a base58 configuration value.
	privStr, err := MarshalPrivateKey(keyPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("private key: %s", privStr)

	// Encode the public key as a base58 configuration value.
	pubStr, err := MarshalPublicKey(keyPub)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("public key: %s", pubStr)

	// Parse the encoded private key.
	privOut, err := ParsePrivateKey(privStr)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the private-key round trip preserves the key.
	if !privOut.Equals(keyPriv) {
		t.Fail()
	}

	// Parse the encoded public key.
	pubOut, err := ParsePublicKey(pubStr)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the public-key round trip preserves the key.
	if !pubOut.Equals(keyPub) {
		t.Fail()
	}

	// Derive the peer identity from the parsed public key.
	id, err := peer.IDFromPublicKey(pubOut)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("peer id: %v", id.String())
}

func testKeyTypes(t *testing.T, testFunc func(*testing.T, crypto.PrivKey, crypto.PubKey)) {
	t.Run("Ed25519", func(t *testing.T) {
		priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		testFunc(t, priv, pub)
	})
}
