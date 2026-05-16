//go:build !js

package hash

import (
	"crypto/sha256"

	// We include sha1 for git support.
	"crypto/sha1" //nolint:gosec

	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
)

func sumHashType(h HashType, data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	case HashType_HashType_SHA1:
		h := sha1.Sum(data) //nolint:gosec
		return h[:], nil
	case HashType_HashType_BLAKE3:
		h := blake3.Sum256(data)
		return h[:], nil
	default:
		return nil, errors.Errorf("hash type unknown: %v", h.String())
	}
}
