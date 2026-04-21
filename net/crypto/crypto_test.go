package crypto

import (
	"crypto/rand"
	"testing"
)

func TestGenerateEd25519Key(t *testing.T) {
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

	_, _, err = GenerateKeyPair(KeyType(999), 0)
	if err != ErrBadKeyType {
		t.Fatalf("expected ErrBadKeyType, got %v", err)
	}
}

func TestKeyPairFromStdKey(t *testing.T) {
	priv, _, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	stdKey, err := PrivKeyToStdKey(priv)
	if err != nil {
		t.Fatal(err)
	}

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
	_, pub, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	encoded := ConfigEncodeKey(data)
	decoded, err := ConfigDecodeKey(encoded)
	if err != nil {
		t.Fatal(err)
	}
	pub2, err := UnmarshalPublicKey(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equals(pub2) {
		t.Fatal("key mismatch after config encode/decode")
	}
}
