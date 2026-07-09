//go:build goscript

package hash

import (
	stdcrypto "crypto"
	"crypto/sha1" //nolint:gosec // Git object storage requires SHA-1.
	"crypto/sha256"
	"hash"
)

func init() {
	stdcrypto.RegisterHash(stdcrypto.SHA1, sha1.New) //nolint:gosec // Git object storage requires SHA-1.
	stdcrypto.RegisterHash(stdcrypto.SHA256, sha256.New)
}

// SupportedHashTypes is the list of built-in hash types.
var SupportedHashTypes = []HashType{
	HashType_HashType_SHA256,
	HashType_HashType_SHA1,
}

// Validate validates the hash type.
func (h HashType) Validate() error {
	switch h {
	case HashType_HashType_UNKNOWN:
		return nil
	case HashType_HashType_SHA256, HashType_HashType_SHA1:
		return nil
	default:
		return newUnsupportedHashTypeError(h, "hash type unsupported in goscript: "+h.String())
	}
}

// GetHashLen returns the hash length.
func (h HashType) GetHashLen() int {
	switch h {
	case HashType_HashType_SHA256:
		return sha256.Size
	case HashType_HashType_SHA1:
		return sha1.Size
	}
	return 0
}

// BuildHasher builds the hasher for the hash type.
func (h HashType) BuildHasher() (hash.Hash, error) {
	switch h {
	case HashType_HashType_SHA256:
		return sha256.New(), nil
	case HashType_HashType_SHA1:
		return sha1.New(), nil
	default:
		return nil, newUnsupportedHashTypeError(h, "hash type unsupported in goscript: "+h.String())
	}
}
