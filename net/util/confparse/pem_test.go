package confparse

import (
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// TestParseKeysPEM tests parsing public and private key pems.
func TestParseKeysPEM(t *testing.T) {
	testKeyTypes(t, testParseKeysPEM)
}

func testParseKeysPEM(t *testing.T, keyPriv crypto.PrivKey, keyPub crypto.PubKey) {
	// Encode the private key as a PEM configuration value.
	privPEM, err := keypem.MarshalPrivKeyPem(keyPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("private key pem: %s", string(privPEM))

	// Encode the public key as a PEM configuration value.
	pubPEM, err := keypem.MarshalPubKeyPem(keyPub)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("public key pem: %s", string(pubPEM))

	// Parse the PEM private key through the configuration parser.
	// parse
	privOut, err := ParsePrivateKey(string(privPEM))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the PEM private-key round trip preserves the key.
	if !privOut.Equals(keyPriv) {
		t.Fail()
	}

	// Parse the PEM public key through the configuration parser.
	// parse
	pubOut, err := ParsePublicKey(string(pubPEM))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the PEM public-key round trip preserves the key.
	if !pubOut.Equals(keyPub) {
		t.Fail()
	}
}
