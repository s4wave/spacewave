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
	privPEM, err := keypem.MarshalPrivKeyPem(keyPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("private key pem: %s", string(privPEM))

	pubPEM, err := keypem.MarshalPubKeyPem(keyPub)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("public key pem: %s", string(pubPEM))

	// parse
	privOut, err := ParsePrivateKey(string(privPEM))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !privOut.Equals(keyPriv) {
		t.Fail()
	}

	// parse
	pubOut, err := ParsePublicKey(string(pubPEM))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !pubOut.Equals(keyPub) {
		t.Fail()
	}
}
