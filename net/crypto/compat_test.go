package crypto

import (
	"crypto/rand"
	"testing"
)

// TestWireCompatibility verifies that our protobuf encoding is byte-compatible
// with go-libp2p's crypto protobuf format. The wire format uses:
// - Field 1 (varint): KeyType enum (0=RSA, 1=Ed25519)
// - Field 2 (bytes): raw key data
//
// Proto2 and proto3 produce identical wire encoding for these simple messages
// when fields have non-default values.
func TestWireCompatibility(t *testing.T) {
	// Ed25519: generate, marshal, verify wire structure.
	priv, pub, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pubBytes, err := MarshalPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	// Ed25519 public key is 32 bytes.
	// Wire format: field 1 (varint 1=Ed25519) + field 2 (32 bytes).
	// Field 1: tag=0x08 (field 1, varint), value=0x01 (Ed25519)
	// Field 2: tag=0x12 (field 2, length-delimited), length=0x20 (32), data
	if len(pubBytes) != 2+2+32 {
		t.Fatalf("expected 36 bytes for Ed25519 pubkey proto, got %d", len(pubBytes))
	}
	if pubBytes[0] != 0x08 || pubBytes[1] != 0x01 {
		t.Fatalf("expected field 1 = Ed25519 (0x08 0x01), got 0x%02x 0x%02x", pubBytes[0], pubBytes[1])
	}
	if pubBytes[2] != 0x12 || pubBytes[3] != 0x20 {
		t.Fatalf("expected field 2 tag+len (0x12 0x20), got 0x%02x 0x%02x", pubBytes[2], pubBytes[3])
	}

	// Verify round-trip.
	pub2, err := UnmarshalPublicKey(pubBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equals(pub2) {
		t.Fatal("round-trip failed for ed25519 public key")
	}

	privBytes, err := MarshalPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := UnmarshalPrivateKey(privBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !priv.Equals(priv2) {
		t.Fatal("round-trip failed for ed25519 private key")
	}
}
