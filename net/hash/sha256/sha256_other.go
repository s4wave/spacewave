//go:build !amd64 || !gc || purego

package sha256

import (
	"crypto/sha256"
	"hash"
)

// New returns a new hash.Hash computing the SHA-256 checksum.
func New() hash.Hash {
	return sha256.New()
}

// Sum256 returns the SHA-256 checksum of data.
func Sum256(data []byte) [Size]byte {
	return sha256.Sum256(data)
}
