package extra25519

import (
	"crypto/sha512"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/ed25519"
)

// PrivateKeyToCurve25519 converts an ed25519 private key into a corresponding
// curve25519 private key such that the resulting curve25519 public key will
// equal the result from PublicKeyToCurve25519.
//
// returns a 64-byte curve25519 scalar
func PrivateKeyToCurve25519(privateKey ed25519.PrivateKey) []byte {
	h := sha512.New()
	h.Write(privateKey[:32])
	digest := h.Sum(nil)

	digest[0] &= 248
	digest[31] &= 127
	digest[31] |= 64

	return digest
}

// PublicKeyToCurve25519 converts an Ed25519 public key into the curve25519
// public key that would be generated from the same private key.
//
// returns a 32-byte curve25519 point and true if valid.
// returns nil, false if not valid
// some ed25519 public keys cannot / should not be converted to curve25519.
func PublicKeyToCurve25519(edBytes ed25519.PublicKey) ([]byte, bool) {
	if IsEdLowOrder(edBytes) {
		return nil, false
	}

	edPoint, err := (&edwards25519.Point{}).SetBytes(edBytes)
	if err != nil {
		return nil, false
	}

	return edPoint.BytesMontgomery(), true
}
