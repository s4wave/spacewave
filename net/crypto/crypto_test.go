package crypto

import (
	"crypto/rand"
	"testing"
)

func TestGenerateEd25519Key(t *testing.T) {
	// Generate an Ed25519 key pair for the cryptographic checks.
	priv, pub, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if priv.Type() != KeyType_Ed25519 {
		t.Fatalf("expected Ed25519, got %v", priv.Type())
	}
	if pub.Type() != KeyType_Ed25519 {
		t.Fatalf("expected Ed25519, got %v", pub.Type())
	}

	// Sign and verify.
	msg := []byte("hello bifrost")
	sig, err := priv.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := pub.Verify(msg, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("signature verification failed")
	}

	// Wrong message should not verify.
	ok, err = pub.Verify([]byte("wrong"), sig)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected verification to fail")
	}
}

func TestMarshalUnmarshalEd25519(t *testing.T) {
	// Generate an Ed25519 key pair for the cryptographic checks.
	priv, pub, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Marshal and unmarshal public key.
	pubBytes, err := MarshalPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pub2, err := UnmarshalPublicKey(pubBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equals(pub2) {
		t.Fatal("public keys not equal after marshal/unmarshal")
	}

	// Marshal and unmarshal private key.
	privBytes, err := MarshalPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := UnmarshalPrivateKey(privBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !priv.Equals(priv2) {
		t.Fatal("private keys not equal after marshal/unmarshal")
	}

	// Cross-verify: sign with original, verify with unmarshaled.
	msg := []byte("cross verify")
	sig, err := priv.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := pub2.Verify(msg, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("cross-verification failed")
	}
}

func TestGenerateKeyPair(t *testing.T) {
	// Verify key pair generation returns matching Ed25519 keys.
	priv, pub, err := GenerateKeyPair(KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	if priv.Type() != KeyType_Ed25519 {
		t.Fatal("wrong type")
	}
	if !priv.GetPublic().Equals(pub) {
		t.Fatal("public key mismatch")
	}

	// Require an unsupported key type to return the key type error.
	_, _, err = GenerateKeyPair(KeyType(999), 0)
	if err != ErrBadKeyType {
		t.Fatalf("expected ErrBadKeyType, got %v", err)
	}
}

func TestKeyPairFromStdKey(t *testing.T) {
	// Generate the private key for the standard library conversion.
	priv, _, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Convert the private key to its standard library representation.
	stdKey, err := PrivKeyToStdKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the standard library key recovers the original key pair.
	priv2, pub2, err := KeyPairFromStdKey(stdKey)
	if err != nil {
		t.Fatal(err)
	}
	if !priv.Equals(priv2) {
		t.Fatal("private keys not equal after round-trip through stdlib")
	}
	if !priv.GetPublic().Equals(pub2) {
		t.Fatal("public keys not equal after round-trip through stdlib")
	}
}

func TestConfigEncodeDecodeKey(t *testing.T) {
	// Generate and marshal a public key for configuration encoding.
	_, pub, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	// Round-trip the public key through its base64 configuration encoding.
	encoded := ConfigEncodeKey(data)
	decoded, err := ConfigDecodeKey(encoded)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the configuration bytes recover the original public key.
	pub2, err := UnmarshalPublicKey(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equals(pub2) {
		t.Fatal("key mismatch after config encode/decode")
	}
}
