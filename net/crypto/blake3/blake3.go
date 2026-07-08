//go:build !goscript

// Package blake3 provides Spacewave's BLAKE3 owner API.
package blake3

import zeebo "github.com/zeebo/blake3"

// Digest captures a BLAKE3 output stream snapshot.
type Digest = zeebo.Digest

// Hasher is a hash.Hash for BLAKE3.
type Hasher = zeebo.Hasher

// DeriveKey derives key material for the context and input material.
func DeriveKey(context string, material []byte, out []byte) {
	zeebo.DeriveKey(context, material, out)
}

// New returns a new unkeyed BLAKE3 hasher.
func New() *Hasher {
	return zeebo.New()
}

// NewDeriveKey returns a BLAKE3 key-derivation hasher for the context.
func NewDeriveKey(context string) *Hasher {
	return zeebo.NewDeriveKey(context)
}

// NewKeyed returns a keyed BLAKE3 hasher.
func NewKeyed(key []byte) (*Hasher, error) {
	return zeebo.NewKeyed(key)
}

// Sum256 returns the first 256 bits of the unkeyed BLAKE3 digest.
func Sum256(data []byte) [32]byte {
	return zeebo.Sum256(data)
}

// Sum512 returns the first 512 bits of the unkeyed BLAKE3 digest.
func Sum512(data []byte) [64]byte {
	return zeebo.Sum512(data)
}
