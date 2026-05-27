//go:build goscript

package hash

import (
	"crypto/sha256"
	"hash"

	"github.com/pkg/errors"
)

// SupportedHashTypes is the list of built-in hash types.
var SupportedHashTypes = []HashType{
	HashType_HashType_SHA256,
}

// Validate validates the hash type.
func (h HashType) Validate() error {
	switch h {
	case HashType_HashType_UNKNOWN:
		return nil
	case HashType_HashType_SHA256:
		return nil
	default:
		return errors.Errorf("hash type unsupported in goscript: %v", h.String())
	}
}

// GetHashLen returns the hash length.
func (h HashType) GetHashLen() int {
	switch h {
	case HashType_HashType_SHA256:
		return sha256.Size
	}
	return 0
}

// BuildHasher builds the hasher for the hash type.
func (h HashType) BuildHasher() (hash.Hash, error) {
	switch h {
	case HashType_HashType_SHA256:
		return sha256.New(), nil
	default:
		return nil, errors.Errorf("hash type unsupported in goscript: %v", h.String())
	}
}
